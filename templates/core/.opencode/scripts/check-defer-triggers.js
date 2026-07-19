// check-defer-triggers.js — predicate evaluator for DEFER / p2 / follow-up
// candidates held in .local/{{COORDINATOR_DIR}}/tasks/.
//
// TWO MODES, ONE EVALUATOR. The predicate primitives (repoRoot, resolveSince,
// changedPathsSince, isSafeRef, tagExists, parsePredicate, evaluatePredicate,
// extractTriggers) are shared. Mode selects the surface contract:
//
//   PROMOTER MODE (default; no --mode flag, or --mode=promoter)
//     - The R6 "mechanized lightly" piece of the curation model (composition
//       O1). DEFER / p2 / follow-up findings land in the holding area as
//       conditional candidates with a Notes provenance block, including a
//       `trigger:` line describing the condition under which the candidate
//       becomes real work. This script reads those candidates and reports
//       which ones' triggers are currently met, so the promoter can apply the
//       Definition of Ready during a promotion cycle.
//     - PROMOTER-USE-ONLY. Run by the promoter during a promotion cycle.
//     - NEVER wired into a commit hook.
//     - NEVER blocking. It prints a human-readable report and exits 0; it does
//       not gate commits, edits, releases, or any other agent action.
//     - Lenient: unknown predicates report as `unknown-predicate` and never
//       throw. This preserves backward compatibility with existing cards whose
//       trigger grammar predates the strict release contract.
//
//   RELEASE MODE (--mode=release)
//     - Strict release-time evaluator. Consumed by the authorized release
//       wrapper (hard refusal) AND by the advisory readiness surface. ONE
//       evaluator so the two surfaces cannot drift.
//     - Emits STRUCTURED JSON (single object) and returns NONZERO for blocker
//       or evaluator-error classifications.
//     - Selects ONLY source:review-defer candidates; source:p2-followup is
//       EXCLUDED from v1 release scope.
//     - REJECTS unsupported trigger grammar (|| separators, directory
//       operands, non-predicate terms, empty args) as evaluator-error — NEVER
//       silently "unfired." Silently treating broken grammar as not-met would
//       be a release bypass.
//     - Fail-closed: malformed JSON, unknown lifecycle status, unreadable
//       tasks dir, missing trigger:/studied: provenance, AND null/indeterminate
//       release arc (unresolvable --since, the HEAD~N fallback when no tag
//       exists, git unusable, bad ref, empty/shallow repo) all produce
//       evaluator-error. Absent or empty tasks dir → clear (pass). An
//       empty-but-deterministic arc (zero changed paths) is NOT null and
//       evaluates normally (no files in scope → no path_touched can fire).
//
// This is a deliberately tiny predicate engine, not a full rules system:
//   path_touched(<path>)   true if <path> appears in `git diff --name-only`
//                          since <since>. EXACT path match in release mode —
//                          no glob, no directory-prefix. <path> ending in `/`
//                          is a directory operand and is evaluator-error in
//                          release mode.
//                          Promoter default <since>: the most recent tag, or
//                          HEAD~32 if no tag exists (bounded fallback so a
//                          fresh repo still produces useful output).
//   after_tag(<tag>)       true if <tag> exists.
//
// A candidate may carry multiple `trigger:` lines (AND semantics in both
// modes) or a single `trigger: any(...)` line (OR of the inner predicates).
// In PROMOTER mode, unknown predicates evaluate to false and are reported as
// `unknown-predicate`, never thrown. In RELEASE mode, unknown predicates are
// evaluator-error (fail closed).
//
// The Notes provenance block this script reads looks like:
//   source:review-defer
//   trigger:path_touched(src/auth/login.go)
//   studied:2026-04-30
//
// USAGE:
//   # Promoter mode (human-readable, always exit 0)
//   node .opencode/scripts/check-defer-triggers.js [--since <ref>] [--tasks <dir>]
//
//   # Release mode (JSON, nonzero on blocker/evaluator-error)
//   node .opencode/scripts/check-defer-triggers.js --mode=release [--since <ref>] [--tasks <dir>]
//
// Promoter-mode failures (missing git, unreadable dir) print a warning line
// and degrade to "no candidates". Release-mode failures fail closed.

import { execFileSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

// ESM has no global __dirname; derive it from import.meta.url (mirrors the
// proven shim in state-lib.js) so repoRoot() is cwd-robust when node is
// spawned by the opencode plugin server / Go bridge with an explicit cwd.
const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);

// repoRoot() is inlined here (zero-dep, mirrors state-lib.js's definition)
// instead of imported, so this MVP predicate checker stays self-contained and
// does not couple to a larger module for one helper.
function repoRoot() {
    return path.resolve(__dirname, "..", "..");
}

// The coordinator dir token is rendered by the harness on `update`; at
// runtime the literal here is the real dir name. Mirrors state-lib.js's
// localCoordinatorRoot() pattern (path.join(repoRoot(), ".local",
// "{{COORDINATOR_DIR}}")).
const COORDINATOR_DIR = "{{COORDINATOR_DIR}}";

function defaultTasksDir() {
    return path.join(repoRoot(), ".local", COORDINATOR_DIR, "tasks");
}

// Split `--flag=value` into [`--flag`, `value`] while leaving `--flag value`
// (two argv slots) untouched. Both forms are accepted for every flag.
function splitLongFlag(a) {
    const idx = a.indexOf("=");
    if (idx > 2 && a.startsWith("--")) {
        return [a.slice(0, idx), a.slice(idx + 1)];
    }
    return [a, null];
}

function parseArgs(argv) {
    const options = {
        since: null, // ref/tag to diff against; null = auto (latest tag or HEAD~32)
        tasksDir: null, // override the tasks dir; null = defaultTasksDir()
        mode: null, // null|'promoter' = human-readable (default); 'release' = JSON + strict
    };
    for (let i = 2; i < argv.length; i++) {
        const raw = argv[i];
        const [a, inlineValue] = splitLongFlag(raw);
        // --flag=value form: consume the inline value; --flag value form: consume next argv slot.
        const takeValue = (fallbackNext) => {
            if (inlineValue !== null) return inlineValue;
            if (fallbackNext && i + 1 < argv.length) return argv[++i];
            return null;
        };
        if (a === "--since") {
            const v = takeValue(true);
            if (v !== null) options.since = v;
        } else if (a === "--tasks") {
            const v = takeValue(true);
            if (v !== null) options.tasksDir = v;
        } else if (a === "--mode") {
            const v = takeValue(true);
            if (v !== null) options.mode = v;
        } else if (a === "--help" || a === "-h") {
            process.stdout.write(
                "usage: check-defer-triggers.js [--mode promoter|release] [--since <ref>] [--tasks <dir>]\n" +
                "  Predicate evaluator for DEFER/p2/follow-up candidates.\n" +
                "  Default (promoter) mode: human-readable report, never blocking, exit 0.\n" +
                "  --mode=release: strict JSON release evaluation; nonzero on blocker or\n" +
                "    evaluator-error. Fail-closed on malformed cards / unknown status /\n" +
                "    unsupported trigger grammar. Absent/empty tasks dir → clear (pass).\n",
            );
            process.exit(0);
        }
    }
    return options;
}

// Resolve --since to a concrete ref. Default policy: most recent tag reachable
// from HEAD; if no tag exists, fall back to HEAD~32 so a fresh repo still
// produces a bounded diff. Returns null only if git itself is unusable.
function resolveSince(options) {
    if (options.since) return options.since;
    try {
        // describe --tags --abbrev=0 gives the nearest tag; ignore failures.
        // execFileSync with argv array — NEVER interpolate into a shell string
        // (defense against injection from operator-supplied --since values).
        const tag = execFileSync(
            "git", ["describe", "--tags", "--abbrev=0"],
            { cwd: repoRoot(), encoding: "utf8", stdio: ["ignore", "pipe", "ignore"] },
        ).trim();
        if (tag) return tag;
    } catch (_) {
        // no tag, or git unavailable — fall through to HEAD~N.
    }
    return "HEAD~32";
}

// Return the set of paths changed since `since` (repo-relative, forward
// slashes). Returns null if git is unusable (caller degrades to "no data").
function changedPathsSince(since) {
    if (!isSafeRef(since)) return null;
    try {
        // execFileSync with argv array — `since` may originate from an
        // operator --since flag; never interpolate it into a shell string.
        const out = execFileSync(
            "git", ["diff", "--name-only", since],
            { cwd: repoRoot(), encoding: "utf8", stdio: ["ignore", "pipe", "ignore"] },
        );
        return new Set(
            out.split("\n")
                .map((l) => l.trim())
                .filter(Boolean),
        );
    } catch (_) {
        return null;
    }
}

// Conservative ref-name validation. Git ref names are restricted to
// [A-Za-z0-9][A-Za-z0-9._/-]* roughly; we enforce a tight allowlist so a
// trigger arg can never carry shell metacharacters even if execFileSync
// were somehow bypassed. Returns true if the arg looks like a safe ref/path.
function isSafeRef(arg) {
    return /^[A-Za-z0-9][A-Za-z0-9._\/-]*$/.test(arg);
}

// True if `tag` exists in the repo. Used by after_tag(). Uses execFileSync
// with an argv array — NEVER shell interpolation — so a malicious trigger
// arg cannot inject commands. isSafeRef is defense-in-depth on top.
function tagExists(tag) {
    if (!isSafeRef(tag)) return false;
    try {
        execFileSync(
            "git", ["rev-parse", "--verify", "--quiet", `refs/tags/${tag}`],
            { cwd: repoRoot(), encoding: "utf8", stdio: ["ignore", "pipe", "ignore"] },
        );
        // rev-parse --verify exits 0 if the ref resolves. We arrived here only
        // if execFileSync did not throw, which means exit 0 -> the tag exists.
        return true;
    } catch (_) {
        return false;
    }
}

// Parse a single predicate string into {kind, arg}. Returns null for
// unrecognized shapes (caller reports unknown-predicate). PROMOTER-mode
// parser: lenient greedy match so `path_touched(a)||path_touched(b)` reports
// unknown-predicate rather than throwing. Release mode uses
// parsePredicateStrict instead.
function parsePredicate(trigger) {
    const t = (trigger || "").trim();
    let m = t.match(/^path_touched\((.+)\)$/);
    if (m) return { kind: "path_touched", arg: m[1].trim() };
    m = t.match(/^after_tag\((.+)\)$/);
    if (m) return { kind: "after_tag", arg: m[1].trim() };
    return null;
}

// STRICT predicate parser for release mode. Returns {kind, arg} on success or
// {error: string} on any unsupported shape. The arg capture is `[^)]+` (NOT
// greedy `.+`) so a `||`-separated chain fails the closing `)\s*$` anchor and
// falls through to "unsupported predicate" rather than silently matching the
// first fragment. This is the key safety property: broken grammar can NEVER
// be misread as a valid predicate that happens not to fire.
function parsePredicateStrict(trigger) {
    const t = (trigger || "").trim();
    if (!t) return { error: "empty trigger term" };
    let m = t.match(/^path_touched\(([^)]+)\)$/);
    if (m) {
        const arg = m[1].trim();
        if (!arg) return { error: "empty path_touched argument" };
        if (arg.endsWith("/")) return { error: `directory operand not supported in v1: ${arg}` };
        return { kind: "path_touched", arg };
    }
    m = t.match(/^after_tag\(([^)]+)\)$/);
    if (m) {
        const arg = m[1].trim();
        if (!arg) return { error: "empty after_tag argument" };
        return { kind: "after_tag", arg };
    }
    return { error: `unsupported predicate: ${t}` };
}

// Evaluate one parsed predicate against the current repo state. Returns
// { met: bool, note: string }. Shared by both modes.
function evaluatePredicate(pred, changedPaths) {
    if (pred.kind === "path_touched") {
        if (!changedPaths) {
            return { met: false, note: "no-git-diff-data" };
        }
        const hit = changedPaths.has(pred.arg);
        return { met: hit, note: hit ? "touched" : "not-touched-since-ref" };
    }
    if (pred.kind === "after_tag") {
        const exists = tagExists(pred.arg);
        return {
            met: exists,
            note: exists ? "tag-exists" : "tag-missing",
        };
    }
    return { met: false, note: "unknown-predicate" };
}

// Collect every `trigger:` line from a task-card body's Notes block. We scan
// the whole file for `^trigger:` lines (the Notes provenance convention);
// lines starting with `trigger:any(` open an OR-group whose members are
// parsed from the comma-separated inner text. PROMOTER-mode extractor: pairs
// with the lenient parsePredicate.
function extractTriggers(body) {
    if (!body || typeof body !== "string") return [];
    const triggers = [];
    const anyRe = /^trigger:\s*any\((.+)\)\s*$/im;
    const anyMatch = body.match(anyRe);
    if (anyMatch) {
        for (const piece of anyMatch[1].split(",")) {
            const t = piece.trim();
            if (t) triggers.push(t);
        }
        return { mode: "any", items: triggers };
    }
    const lineRe = /^trigger:\s*(.+?)\s*$/gim;
    let m;
    while ((m = lineRe.exec(body)) !== null) {
        triggers.push(m[1].trim());
    }
    return { mode: "all", items: triggers };
}

// STRICT trigger extractor for release mode. Mirrors extractTriggers' shape
// detection (any() OR-group vs line-by-line AND) but pairs each item with a
// parsePredicateStrict result and surfaces the FIRST grammar error. Returns:
//   { mode: "any"|"all", items: string[], details: per-item{},
//     hasTriggerLine: bool, grammarError: string|null }
// `details` entries: { raw, predicate, arg, fired, note } on success or
// { raw, predicate:null, arg:null, fired:false, note:<error> } on grammar
// error. The candidate is evaluator-error if grammarError is set, regardless
// of how many other items parsed cleanly.
function extractTriggersRelease(notesText, changedPaths) {
    const result = {
        mode: "all",
        items: [],
        details: [],
        hasTriggerLine: false,
        grammarError: null,
    };
    if (!notesText || typeof notesText !== "string") return result;

    const anyRe = /^trigger:\s*any\((.+)\)\s*$/im;
    const anyMatch = notesText.match(anyRe);
    if (anyMatch) {
        result.hasTriggerLine = true;
        result.mode = "any";
        for (const piece of anyMatch[1].split(",")) {
            const t = piece.trim();
            if (!t) {
                if (!result.grammarError) result.grammarError = "empty member in trigger:any()";
                continue;
            }
            result.items.push(t);
        }
    } else {
        const lineRe = /^trigger:\s*(.+?)\s*$/gim;
        let m;
        while ((m = lineRe.exec(notesText)) !== null) {
            const t = m[1].trim();
            if (t) {
                result.hasTriggerLine = true;
                result.items.push(t);
            }
        }
    }

    for (const t of result.items) {
        const parsed = parsePredicateStrict(t);
        if (parsed.error) {
            if (!result.grammarError) result.grammarError = parsed.error;
            result.details.push({ raw: t, predicate: null, arg: null, fired: false, note: parsed.error });
            continue;
        }
        const ev = evaluatePredicate(parsed, changedPaths);
        result.details.push({
            raw: t,
            predicate: parsed.kind,
            arg: parsed.arg,
            fired: !!ev.met,
            note: ev.note,
        });
    }
    return result;
}

// Evaluate one candidate (PROMOTER mode). Returns a report object. `body` is
// the PARSED JSON task-card object (not the raw file text): task_id and
// owner_notes are read natively so DEFER/p2-followup cards (.json produced
// by /write-task) are honored. The Notes-prefix trigger grammar is fed
// UNMODIFIED to extractTriggers as the owner_notes[] text joined by newlines
// — the existing `^trigger:` regex + predicate parser are unchanged.
function evaluateCandidate(file, body, since, changedPaths) {
    const id = (body && typeof body.task_id === "string" && body.task_id)
        || path.basename(file, ".json");
    const notesText = (body && Array.isArray(body.owner_notes))
        ? body.owner_notes.join("\n")
        : "";
    const trig = extractTriggers(notesText);
    if (!trig.items || trig.items.length === 0) {
        return { id, file, met: false, mode: "none", note: "no-trigger-line", details: [] };
    }
    const details = trig.items.map((t) => {
        const pred = parsePredicate(t);
        if (!pred) return { trigger: t, met: false, note: "unknown-predicate" };
        return { trigger: t, ...evaluatePredicate(pred, changedPaths) };
    });
    const met = trig.mode === "any"
        ? details.some((d) => d.met)
        : details.every((d) => d.met);
    return { id, file, met, mode: trig.mode, note: met ? "ready-for-dor" : "trigger-not-met", details };
}

// ---- Release-mode primitives (strict) -------------------------------------

// Lifecycle resolution map for release mode. `completed`/`cancelled` are
// resolved; `draft`/`ready`/`working`/`reported`/`blocked` are unresolved;
// anything else is unknown → evaluator-error (fail closed; NEVER implicit
// pass). Returns "resolved" | "unresolved" | null.
const RELEASE_RESOLVED_STATUSES = new Set(["completed", "cancelled"]);
const RELEASE_UNRESOLVED_STATUSES = new Set(["draft", "ready", "working", "reported", "blocked"]);
function releaseResolutionOf(status) {
    if (RELEASE_RESOLVED_STATUSES.has(status)) return "resolved";
    if (RELEASE_UNRESOLVED_STATUSES.has(status)) return "unresolved";
    return null;
}

// Probe the tasks directory without throwing. Returns one of:
//   "absent"     — does not exist (clear; absence is not a mandatory surface)
//   "empty"      — exists but contains no .json task cards (clear)
//   "present"    — exists and has ≥1 .json file (evaluate each)
//   "unreadable" — exists but stat/readdir failed (fail closed)
function probeTasksDir(tasksDir) {
    let stat;
    try {
        stat = fs.statSync(tasksDir);
    } catch (e) {
        if (e && e.code === "ENOENT") return "absent";
        return "unreadable";
    }
    if (!stat.isDirectory()) return "unreadable";
    let entries;
    try {
        entries = fs.readdirSync(tasksDir);
    } catch (_) {
        return "unreadable";
    }
    return entries.some((f) => f.endsWith(".json")) ? "present" : "empty";
}

// Evaluate one candidate (RELEASE mode). `body` is the PARSED JSON object or
// null (null is handled by the caller as malformed-json). Returns a finding
// object, or null if the card is not a release-defer candidate (no
// source:review-defer in owner_notes). The caller MUST treat a null return
// as "skip silently" — non-candidate cards do not appear in findings.
function evaluateCandidateRelease(file, body, since, changedPaths) {
    const filename = path.basename(file);
    const fallbackId = filename.replace(/\.json$/, "");
    const id = (body && typeof body.task_id === "string" && body.task_id) || fallbackId;

    const notes = (body && Array.isArray(body.owner_notes)) ? body.owner_notes.slice() : [];
    const notesText = notes.map((n) => (typeof n === "string" ? n : String(n))).join("\n");

    // Source selection: ONLY source:review-defer is a release candidate.
    // source:p2-followup is excluded from v1 release scope.
    const hasReviewDefer = notes.some((n) => typeof n === "string" && n.trim() === "source:review-defer");
    if (!hasReviewDefer) return null;

    const status = (body && typeof body.status === "string") ? body.status : "";
    const resolution = releaseResolutionOf(status);
    if (resolution === null) {
        return {
            id, file, source: "review-defer", status, resolution: null,
            trigger_grammar: "unknown-status", triggers: [], fired: false,
            release_relevant: false, classification: "evaluator-error",
            reason: `unknown lifecycle status: ${JSON.stringify(status)}`,
        };
    }

    const hasStudied = notes.some((n) => typeof n === "string" && /^studied:/.test(n.trim()));
    const trig = extractTriggersRelease(notesText, changedPaths);

    if (!trig.hasTriggerLine) {
        return {
            id, file, source: "review-defer", status, resolution,
            trigger_grammar: "missing-trigger", triggers: [], fired: false,
            release_relevant: false, classification: "evaluator-error",
            reason: "missing trigger: provenance line",
        };
    }
    if (!hasStudied) {
        return {
            id, file, source: "review-defer", status, resolution,
            trigger_grammar: "missing-studied", triggers: trig.details, fired: false,
            release_relevant: false, classification: "evaluator-error",
            reason: "missing studied: provenance line",
        };
    }
    if (trig.grammarError) {
        return {
            id, file, source: "review-defer", status, resolution,
            trigger_grammar: "unsupported", triggers: trig.details, fired: false,
            release_relevant: false, classification: "evaluator-error",
            reason: `unsupported trigger grammar: ${trig.grammarError}`,
        };
    }

    const fired = trig.mode === "any"
        ? trig.details.some((d) => d.fired)
        : trig.details.every((d) => d.fired);

    if (resolution === "resolved") {
        return {
            id, file, source: "review-defer", status, resolution,
            trigger_grammar: "valid", triggers: trig.details, fired,
            release_relevant: false, classification: "clear",
            reason: fired ? "resolved-and-fired" : "resolved-and-unfired",
        };
    }
    // unresolved
    if (fired) {
        return {
            id, file, source: "review-defer", status, resolution,
            trigger_grammar: "valid", triggers: trig.details, fired: true,
            release_relevant: true, classification: "blocker",
            reason: "fired-and-unresolved",
        };
    }
    return {
        id, file, source: "review-defer", status, resolution,
        trigger_grammar: "valid", triggers: trig.details, fired: false,
        release_relevant: false, classification: "advisory",
        reason: "unfired-and-unresolved",
    };
}

// Lexicographic comparator for deterministic finding/ID ordering.
function lexCompare(a, b) {
    if (a < b) return -1;
    if (a > b) return 1;
    return 0;
}

// Emit the release-mode JSON envelope and exit with the classification's
// canonical exit code: clear/advisory → 0; blocker → 1; evaluator-error → 2.
function emitReleaseResult(payload) {
    process.stdout.write(JSON.stringify(payload, null, 2) + "\n");
    const code = payload.classification === "blocker" ? 1
        : payload.classification === "evaluator-error" ? 2 : 0;
    process.exit(code);
}

// RELEASE-mode entrypoint. Builds the strict JSON envelope.
function mainRelease(options) {
    const tasksDir = options.tasksDir ? path.resolve(options.tasksDir) : defaultTasksDir();
    const since = resolveSince(options);

    const state = probeTasksDir(tasksDir);

    // Compute the release arc's changed paths whenever git is usable, even
    // when there are no candidates — the arc is surfaced in the output for
    // diagnostics and so the consuming release surfaces can show what was evaluated.
    const changedPaths = changedPathsSince(since);
    const arcPaths = changedPaths ? Array.from(changedPaths).sort(lexCompare) : [];

    const baseEnvelope = {
        mode: "release",
        since,
        tasks_dir: tasksDir,
        tasks_dir_state: state,
        arc_paths: arcPaths,
        findings: [],
        blocking_ids: [],
        advisory_ids: [],
        evaluator_error_ids: [],
        resolved_ids: [],
        error: null,
    };

    // Fail-closed: a null/indeterminate release arc MUST be evaluator-error
    // BEFORE candidate evaluation. changedPathsSince returns null when
    // isSafeRef rejects the resolved --since (notably the HEAD~N fallback when
    // no tag exists — `~` is outside the safe-ref charset) OR when git is
    // unusable / the ref is bad / the repo is empty or shallow. In ALL these
    // cases the arc cannot be deterministically computed, so we CANNOT make
    // any determination about path_touched predicates. Refusing here prevents
    // the fail-open path where a fired path_touched candidate is silently
    // downgraded to advisory (exit 0) and the wrapper proceeds to `git tag`.
    //
    // An EMPTY arc (Set with 0 entries — `git diff --name-only <tag>` ran
    // successfully and returned no paths) is NOT null: changedPaths is a
    // truthy Set object, so this check does not fire, and evaluation
    // continues normally (no files in scope → no path_touched can fire →
    // clear/advisory). This preserves the empty-arc-pass vs null-arc-error
    // distinction.
    if (!changedPaths) {
        baseEnvelope.classification = "evaluator-error";
        baseEnvelope.error = `release arc indeterminate: cannot compute changed paths since ${since}`;
        emitReleaseResult(baseEnvelope);
        return;
    }

    if (state === "absent" || state === "empty") {
        baseEnvelope.classification = "clear";
        emitReleaseResult(baseEnvelope);
        return; // emitReleaseResult exits
    }
    if (state === "unreadable") {
        baseEnvelope.classification = "evaluator-error";
        baseEnvelope.error = `tasks dir unreadable: ${tasksDir}`;
        emitReleaseResult(baseEnvelope);
        return;
    }

    // state === "present" — evaluate each .json file.
    const entries = fs.readdirSync(tasksDir)
        .filter((f) => f.endsWith(".json"))
        .sort(lexCompare);
    const findings = [];
    for (const name of entries) {
        const file = path.join(tasksDir, name);
        let body = null;
        let parseFailed = false;
        try {
            body = JSON.parse(fs.readFileSync(file, "utf8"));
        } catch (_) {
            parseFailed = true;
        }
        if (parseFailed || body === null || typeof body !== "object" || Array.isArray(body)) {
            findings.push({
                id: name.replace(/\.json$/, ""),
                file,
                source: null,
                status: null,
                resolution: null,
                trigger_grammar: "malformed-json",
                triggers: [],
                fired: false,
                release_relevant: false,
                classification: "evaluator-error",
                reason: "malformed JSON (not a parseable task-card object)",
            });
            continue;
        }
        const finding = evaluateCandidateRelease(file, body, since, changedPaths);
        if (finding) findings.push(finding);
    }

    findings.sort((a, b) => lexCompare(a.id, b.id));

    const blockingIds = findings.filter((f) => f.classification === "blocker").map((f) => f.id);
    const advisoryIds = findings.filter((f) => f.classification === "advisory").map((f) => f.id);
    const errIds = findings.filter((f) => f.classification === "evaluator-error").map((f) => f.id);
    const resolvedIds = findings.filter((f) => f.classification === "clear").map((f) => f.id);

    // Precedence: evaluator-error > blocker > advisory > clear.
    let classification = "clear";
    if (errIds.length > 0) classification = "evaluator-error";
    else if (blockingIds.length > 0) classification = "blocker";
    else if (advisoryIds.length > 0) classification = "advisory";

    baseEnvelope.classification = classification;
    baseEnvelope.findings = findings;
    baseEnvelope.blocking_ids = blockingIds;
    baseEnvelope.advisory_ids = advisoryIds;
    baseEnvelope.evaluator_error_ids = errIds;
    baseEnvelope.resolved_ids = resolvedIds;
    if (classification === "evaluator-error") {
        baseEnvelope.error = `${errIds.length} evaluator-error finding(s): ${errIds.join(", ")}`;
    } else if (classification === "blocker") {
        baseEnvelope.error = `${blockingIds.length} blocking release-defer finding(s): ${blockingIds.join(", ")}`;
    }

    emitReleaseResult(baseEnvelope);
}

// ---- Promoter-mode entrypoint (UNCHANGED behavior) -----------------------

function mainPromoter(options) {
    const tasksDir = options.tasksDir ? path.resolve(options.tasksDir) : defaultTasksDir();
    const since = resolveSince(options);

    let files = [];
    try {
        files = fs
            .readdirSync(tasksDir)
            .filter((f) => f.endsWith(".json"))
            .map((f) => path.join(tasksDir, f));
    } catch (_) {
        process.stdout.write(
            `check-defer-triggers: no tasks dir at ${tasksDir} (or unreadable). ` +
            `Nothing to evaluate. Promoter-use-only; never blocking.\n`,
        );
        process.exit(0);
    }

    const changedPaths = changedPathsSince(since);
    const sinceNote = changedPaths
        ? `diff-since=${since} (${changedPaths.size} changed paths)`
        : `diff-since=${since} (git unavailable — predicates degrade to not-met)`;

    const reports = files.map((f) => {
        let body = null;
        try {
            const raw = fs.readFileSync(f, "utf8");
            body = JSON.parse(raw);
        } catch (_) {
            body = null;
        }
        return evaluateCandidate(f, body, since, changedPaths);
    });

    process.stdout.write(
        `check-defer-triggers report — promoter-use-only, never blocking.\n` +
        `tasks-dir: ${tasksDir}\n` +
        `${sinceNote}\n\n`,
    );

    if (reports.length === 0) {
        process.stdout.write("No candidates found.\n");
        process.exit(0);
    }

    for (const r of reports) {
        const flag = r.met ? "READY" : "hold";
        process.stdout.write(`[${flag}] ${r.id} (${path.basename(r.file)}) — ${r.note}\n`);
        for (const d of r.details) {
            const mark = d.met ? "met" : "not-met";
            process.stdout.write(`    ${mark}: ${d.trigger} (${d.note})\n`);
        }
    }

    const ready = reports.filter((r) => r.met).length;
    process.stdout.write(
        `\n${ready}/${reports.length} candidate(s) have triggers met. ` +
        `Promoter: apply the Definition of Ready (area + file scope + validation ` +
        `plan + clear slice + provenance) before promoting any READY candidate.\n`,
    );
    process.exit(0);
}

// Dispatcher: --mode=release routes to the strict JSON evaluator; anything
// else (null, "promoter", or a typo) routes to the lenient human-readable
// promoter mode. An unknown --mode value is treated as promoter with a stderr
// note (NEVER as release — release semantics must be opt-in).
function main() {
    const options = parseArgs(process.argv);
    if (options.mode === "release") {
        mainRelease(options);
        return;
    }
    if (options.mode !== null && options.mode !== "promoter") {
        process.stderr.write(
            `check-defer-triggers: unknown --mode '${options.mode}', falling back to promoter mode.\n`,
        );
    }
    mainPromoter(options);
}

main();
