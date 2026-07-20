// check-defer-triggers.js — predicate evaluator for DEFER / p2 / follow-up
// candidates held in .local/coordinator/tasks/.
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
//     - Strict releasetime evaluator. Consumed by the authorized release
//       wrapper (hard refusal) AND by the advisory readiness surface. ONE
//       evaluator so the two surfaces cannot drift.
//     - Emits STRUCTURED JSON (single object) and returns NONZERO for blocker
//       or evaluator-error classifications.
//     - TWO input modes, selected by the RELEASE_DEFER_MANIFEST_AUTHORITY env:
//       LEGACY (env unset): reads .local/coordinator/tasks/ as before.
//         Kept byte-identical for backward compatibility during the rollout.
//       MANIFEST AUTHORITY (env=1|true): reads the committed manifest at
//         .vh-agent-harness/release-defer-dispositions.json ONLY. Performs NO
//         .local/ access whatsoever. The committed manifest is the release
//         truth; .local/ stays promoter/provenance transport.
//     - LEGACY selects ONLY source:review-defer candidates; source:p2-followup
//       is EXCLUDED from v1 release scope. LEGACY REJECTS unsupported trigger
//       grammar (|| separators, directory operands, non-predicate terms, empty
//       args) as evaluator-error — NEVER silently "unfired." Silently treating
//       broken grammar as not-met would be a release bypass.
//     - LEGACY fail-closed: malformed JSON, unknown lifecycle status,
//       unreadable tasks dir, missing trigger:/studied: provenance, AND
//       null/indeterminate release arc (unresolvable --since, the HEAD~N
//       fallback when no tag exists, git unusable, bad ref, empty/shallow repo)
//       all produce evaluator-error. Absent or empty tasks dir → clear (pass).
//       An empty-but-deterministic arc (zero changed paths) is NOT null and
//       evaluates normally (no files in scope → no path_touched can fire).
//     - MANIFEST AUTHORITY fail-closed: missing/malformed manifest, unsupported
//       schema_version, unknown enum values, duplicate IDs, unsorted records,
//       handshake mismatch (evaluated_commit/manifest_parent_commit/HEAD^,
//       evaluated_tree/HEAD^{tree}, diff != [manifest path only]), wrong
//       release_base, empty records without reconciliation.zero_records_confirmed,
//       and any disposition-matrix refusal ALL produce blocker or
//       evaluator-error. The handshake (sacred — do not weaken) prevents the
//       manifest from being weakened after its claimed evaluation.
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
//   # Release mode — LEGACY (reads .local/; backward compat)
//   node .opencode/scripts/check-defer-triggers.js --mode=release [--since <ref>] [--tasks <dir>]
//
//   # Release mode — MANIFEST AUTHORITY (reads committed manifest ONLY)
//   RELEASE_DEFER_MANIFEST_AUTHORITY=1 \
//   node .opencode/scripts/check-defer-triggers.js --mode=release \
//       [--release-version <vX.Y.Z>] [--override-confirmed-version <vX.Y.Z>]
//
//   --override-confirmed-version is supplied by the authorized release wrapper
//   ONLY after the operator-side override ceremony succeeds
//   (--override-release-version + --override-manifest-sha agree with the
//   requested version and the actual manifest blob SHA). It is the
//   transition-authority signal for Layer B (operator live intent). Layer A
//   (object validity + version match) is verifiable from the committed manifest
//   alone and is enforced with or without the flag; the wrapper adds a post-
//   evaluator gate so an override accepted by Layer A still refuses at tag
//   time when the operator did not supply the ceremony flags. CI verifies
//   Layer A from the committed manifest and accepts a well-formed override
//   without the flag (CI is defense-in-depth, not operator-intent re-enforcement).
//   Model/reviewer surfaces (the advisory readiness surface) cannot supply
//   this flag.
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
// "coordinator")).
const COORDINATOR_DIR = "coordinator";

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
        releaseVersion: null, // release version being tagged (manifest authority: override binding)
        overrideConfirmedVersion: null, // operator-confirmed version (wrapper ceremony); honors override_required
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
        } else if (a === "--release-version") {
            const v = takeValue(true);
            if (v !== null) options.releaseVersion = v;
        } else if (a === "--override-confirmed-version") {
            const v = takeValue(true);
            if (v !== null) options.overrideConfirmedVersion = v;
        } else if (a === "--help" || a === "-h") {
            process.stdout.write(
                "usage: check-defer-triggers.js [--mode promoter|release] [--since <ref>] [--tasks <dir>]\n" +
                "                                  [--release-version <vX.Y.Z>]\n" +
                "                                  [--override-confirmed-version <vX.Y.Z>]\n" +
                "  Predicate evaluator for DEFER/p2/follow-up candidates.\n" +
                "  Default (promoter) mode: human-readable report, never blocking, exit 0.\n" +
                "  --mode=release: strict JSON release evaluation; nonzero on blocker or\n" +
                "    evaluator-error. Fail-closed on malformed cards / unknown status /\n" +
                "    unsupported trigger grammar. Absent/empty tasks dir → clear (pass).\n" +
                "  RELEASE_DEFER_MANIFEST_AUTHORITY=1: release mode reads the committed\n" +
                "    manifest at .vh-agent-harness/release-defer-dispositions.json ONLY\n" +
                "    (no .local/ access); --release-version binds the release being tagged.\n" +
                "    --override-confirmed-version is the operator-side wrapper confirmation\n" +
                "    signal: an override_required record is honored only when\n" +
                "    override.release_version == --release-version == --override-confirmed-version.\n",
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

// ---- Release-mode MANIFEST AUTHORITY primitives ----------------------------
//
// When RELEASE_DEFER_MANIFEST_AUTHORITY is set (1|true), release mode reads
// the committed manifest at <repoRoot>/.vh-agent-harness/release-defer-
// dispositions.json ONLY and performs NO .local/ access. The committed
// manifest is the release truth. This eliminates the fail-open defect where
// absent .local/ was treated as "clear" and could not protect fresh checkouts.
//
// The manifest carries the operator/promoter's attested disposition for each
// DEFER finding in the declared release arc. The evaluator verifies the
// freshness handshake (manifest commits only itself on top of the evaluated
// commit), validates schema v1, and applies the disposition matrix.

// Release manifest runtime path. This is the harness-conventional config dir
// (stable across installs — not a project domain literal), so it is safe to
// hardcode in templates/core/. The MANIFEST CONTENT is project-owned; the
// PATH is harness convention. Forward-slash form matches git diff output.
const RELEASE_MANIFEST_REL = ".vh-agent-harness/release-defer-dispositions.json";
const RELEASE_MANIFEST_SCHEMA_VERSION = 1;
const RELEASE_RELEVANCE_VALUES = new Set(["yes", "no", "unknown"]);
const RELEASE_DISPOSITION_VALUES = new Set(["block", "disclose", "override_required"]);
const RELEASE_METADATA_VALUES = new Set(["valid", "stale", "invalid"]);

function releaseManifestAbsPath() {
    return path.join(repoRoot(), RELEASE_MANIFEST_REL);
}

// True when release mode should read the committed manifest instead of .local/.
function releaseManifestAuthorityActive() {
    const v = process.env.RELEASE_DEFER_MANIFEST_AUTHORITY;
    return v === "1" || v === "true";
}

// Full 40-char lowercase hex SHA validator.
function isFullSha(s) {
    return typeof s === "string" && /^[0-9a-f]{40}$/.test(s);
}

// Read the bytes of a path AS COMMITTED AT HEAD via `git show HEAD:<path>`.
// This is the manifest-authority contract: the bytes evaluated equal the bytes
// a fresh checkout (and CI) will see — NOT the worktree, which may carry
// uncommitted edits that would otherwise let a dirty edit flip a `block`
// record to `disclose+valid` while leaving the handshake SHAs intact. Returns
// the file content as a UTF-8 string, or null if git fails (typically because
// the path is not tracked at HEAD).
function gitShowHeadBlob(relPath) {
    try {
        return execFileSync(
            "git", ["show", `HEAD:${relPath}`],
            { cwd: repoRoot(), encoding: "utf8", stdio: ["ignore", "pipe", "ignore"] },
        );
    } catch (_) {
        return null;
    }
}

// Compute the git blob SHA of `HEAD:<relPath>` via `git rev-parse`. Returns the
// 40-char lowercase hex blob SHA the committed tree carries, or null on
// failure. The SHA is the override-binding token: it MUST be derived from the
// committed blob (what CI sees), NEVER from a `git hash-object` of the worktree
// path (which a dirty edit could swap out from under the ceremony).
function gitHeadBlobSha(relPath) {
    try {
        const sha = execFileSync(
            "git", ["rev-parse", `HEAD:${relPath}`],
            { cwd: repoRoot(), encoding: "utf8", stdio: ["ignore", "pipe", "ignore"] },
        ).trim();
        return /^[0-9a-f]{40}$/.test(sha) ? sha : null;
    } catch (_) {
        return null;
    }
}

// Resolve HEAD^ to a full commit SHA. Returns null if HEAD^ does not exist
// (single-commit repo) or git is unusable.
function gitHeadParent() {
    try {
        const sha = execFileSync(
            "git", ["rev-parse", "--verify", "--quiet", "HEAD^"],
            { cwd: repoRoot(), encoding: "utf8", stdio: ["ignore", "pipe", "ignore"] },
        ).trim();
        return isFullSha(sha) ? sha : null;
    } catch (_) {
        return null;
    }
}

// Resolve HEAD^^{tree} to the tree SHA of HEAD^ (the evaluated commit P).
// NB: `HEAD^{tree}` would be the tree of HEAD itself (the manifest commit M);
// we need the tree of HEAD^ (P), so the ref is `HEAD^^{tree}` (parent of HEAD,
// peeled to its tree). Forward brace — argv form, no shell, so `^{}` is safe.
function gitHeadParentTree() {
    try {
        const sha = execFileSync(
            "git", ["rev-parse", "--verify", "--quiet", "HEAD^^{tree}"],
            { cwd: repoRoot(), encoding: "utf8", stdio: ["ignore", "pipe", "ignore"] },
        ).trim();
        return isFullSha(sha) ? sha : null;
    } catch (_) {
        return null;
    }
}

// Sorted array of files changed in HEAD^..HEAD, or null on failure. Forward
// slashes (git's output convention) for cross-platform comparability.
function gitDiffHeadRange() {
    try {
        const out = execFileSync(
            "git", ["diff", "--name-only", "HEAD^..HEAD"],
            { cwd: repoRoot(), encoding: "utf8", stdio: ["ignore", "pipe", "ignore"] },
        );
        return out.split("\n").map((l) => l.trim()).filter(Boolean).sort(lexCompare);
    } catch (_) {
        return null;
    }
}

// The most recent reachable tag, or null if none exists. Used to validate
// release_base.kind=tag against the discovered prior tag.
//
// When `excludeVersion` is supplied (CI post-tag recheck forwards the just-cut
// release tag via --release-version), that exact version is excluded from the
// lookup so the function returns the PRIOR tag — the one the manifest's
// release_base.value names. The wrapper's pre-tag invocation also forwards
// --release-version, but the new tag does not exist yet at that point, so the
// exclusion is a no-op there and the bare describe path remains equivalent.
function gitLatestTag(excludeVersion) {
    try {
        if (excludeVersion) {
            // List tags REACHABLE from HEAD in descending version order and
            // return the first that is not the excluded version. The
            // `--merged HEAD` filter is the reachability guarantee: only tags
            // whose commits are ancestors of (or equal to) HEAD are listed.
            // Without it, a maintenance-branch release (e.g. v1.0.2 declaring
            // release_base v1.0.0) would incorrectly select an unrelated
            // higher mainline tag (v1.1.0 on main, unreachable from the
            // maintenance branch HEAD), emit release_base mismatch, and CI's
            // set -e would block GoReleaser publication. `--sort=-v:refname`
            // is version-aware so v0.10.0 > v0.9.0 as expected. This handles
            // both the linear CI post-tag case (the just-cut release tag is
            // now the most recent reachable tag and the manifest's
            // release_base names the prior reachable release) and the branched
            // maintenance-release case.
            const out = execFileSync(
                "git", ["tag", "--merged", "HEAD", "--sort=-v:refname"],
                { cwd: repoRoot(), encoding: "utf8", stdio: ["ignore", "pipe", "ignore"] },
            );
            for (const line of out.split("\n")) {
                const t = line.trim();
                if (t && t !== excludeVersion) {
                    return t;
                }
            }
            return null;
        }
        const tag = execFileSync(
            "git", ["describe", "--tags", "--abbrev=0"],
            { cwd: repoRoot(), encoding: "utf8", stdio: ["ignore", "pipe", "ignore"] },
        ).trim();
        return tag || null;
    } catch (_) {
        return null;
    }
}

// Validate a manifest override object. Returns {ok:true} or {ok:false,error}.
function validateOverrideObject(o, idx) {
    const where = `records[${idx}].override`;
    if (!o || typeof o !== "object" || Array.isArray(o)) {
        return { ok: false, error: `${where} must be an object` };
    }
    if (typeof o.release_version !== "string" || !o.release_version) {
        return { ok: false, error: `${where}.release_version must be a non-empty string` };
    }
    if (typeof o.approved_by !== "string" || !o.approved_by) {
        return { ok: false, error: `${where}.approved_by must be a non-empty string` };
    }
    if (typeof o.approved_at !== "string" || !o.approved_at) {
        return { ok: false, error: `${where}.approved_at must be a non-empty string` };
    }
    if (typeof o.reason !== "string" || !o.reason) {
        return { ok: false, error: `${where}.reason must be a non-empty string` };
    }
    return { ok: true };
}

// Validate the parsed manifest object against schema v1. Returns {ok:true} or
// {ok:false,error:"..."}. Checks schema_version, release_base, evaluated_*,
// manifest_parent_commit, reconciliation, per-record shape + enums + sort +
// duplicate IDs + empty-records rule.
function validateReleaseManifest(obj) {
    if (obj === null || typeof obj !== "object" || Array.isArray(obj)) {
        return { ok: false, error: "manifest root must be a JSON object" };
    }
    if (obj.schema_version !== RELEASE_MANIFEST_SCHEMA_VERSION) {
        return { ok: false, error: `unsupported schema_version: ${JSON.stringify(obj.schema_version)}` };
    }
    const rb = obj.release_base;
    if (!rb || typeof rb !== "object" || Array.isArray(rb)) {
        return { ok: false, error: "release_base must be an object" };
    }
    if (rb.kind !== "root" && rb.kind !== "tag") {
        return { ok: false, error: `release_base.kind must be root|tag; got ${JSON.stringify(rb.kind)}` };
    }
    if (rb.kind === "root") {
        if (rb.value !== null && rb.value !== undefined) {
            return { ok: false, error: "release_base.kind=root requires value null" };
        }
    } else {
        if (typeof rb.value !== "string" || !rb.value) {
            return { ok: false, error: "release_base.kind=tag requires non-empty string value" };
        }
    }
    if (!isFullSha(obj.evaluated_commit)) {
        return { ok: false, error: "evaluated_commit must be a 40-char lowercase hex SHA" };
    }
    if (!isFullSha(obj.evaluated_tree)) {
        return { ok: false, error: "evaluated_tree must be a 40-char lowercase hex SHA" };
    }
    if (!isFullSha(obj.manifest_parent_commit)) {
        return { ok: false, error: "manifest_parent_commit must be a 40-char lowercase hex SHA" };
    }
    const rec = obj.reconciliation;
    if (!rec || typeof rec !== "object" || Array.isArray(rec)) {
        return { ok: false, error: "reconciliation must be an object" };
    }
    if (typeof rec.status !== "string" || !rec.status) {
        return { ok: false, error: "reconciliation.status must be a non-empty string" };
    }
    if (typeof rec.scope !== "string" || !rec.scope) {
        return { ok: false, error: "reconciliation.scope must be a non-empty string" };
    }
    if (typeof rec.zero_records_confirmed !== "boolean") {
        return { ok: false, error: "reconciliation.zero_records_confirmed must be boolean" };
    }
    if (!Array.isArray(obj.records)) {
        return { ok: false, error: "records must be an array" };
    }
    if (obj.records.length === 0 && !rec.zero_records_confirmed) {
        return { ok: false, error: "empty records require reconciliation.zero_records_confirmed=true" };
    }
    const seenIds = new Set();
    for (let i = 0; i < obj.records.length; i++) {
        const r = obj.records[i];
        const where = `records[${i}]`;
        if (!r || typeof r !== "object" || Array.isArray(r)) {
            return { ok: false, error: `${where} must be an object` };
        }
        if (typeof r.defer_id !== "string" || !r.defer_id) {
            return { ok: false, error: `${where}.defer_id must be a non-empty string` };
        }
        if (seenIds.has(r.defer_id)) {
            return { ok: false, error: `duplicate defer_id: ${r.defer_id}` };
        }
        seenIds.add(r.defer_id);
        if (!RELEASE_RELEVANCE_VALUES.has(r.release_relevance)) {
            return { ok: false, error: `${where}.release_relevance invalid: ${JSON.stringify(r.release_relevance)}` };
        }
        if (!RELEASE_DISPOSITION_VALUES.has(r.disposition)) {
            return { ok: false, error: `${where}.disposition invalid: ${JSON.stringify(r.disposition)}` };
        }
        if (!RELEASE_METADATA_VALUES.has(r.metadata_state)) {
            return { ok: false, error: `${where}.metadata_state invalid: ${JSON.stringify(r.metadata_state)}` };
        }
        if (typeof r.summary !== "string" || !r.summary) {
            return { ok: false, error: `${where}.summary must be a non-empty string` };
        }
        if (typeof r.reason !== "string" || !r.reason) {
            return { ok: false, error: `${where}.reason must be a non-empty string` };
        }
        if (typeof r.source_ref !== "string" || !r.source_ref) {
            return { ok: false, error: `${where}.source_ref must be a non-empty string (provenance text; never dereferenced)` };
        }
        if (typeof r.studied_at !== "string" || !r.studied_at) {
            return { ok: false, error: `${where}.studied_at must be a non-empty string` };
        }
        if (typeof r.reviewed_at !== "string" || !r.reviewed_at) {
            return { ok: false, error: `${where}.reviewed_at must be a non-empty string` };
        }
        if (r.override !== null && r.override !== undefined) {
            const ov = validateOverrideObject(r.override, i);
            if (!ov.ok) return ov;
        }
    }
    // Sort check: records must already be in lexical order by defer_id. This
    // forces deterministic authoring and prevents reorderings from masking a
    // duplicate or a sneaked-in record.
    const ids = obj.records.map((r) => r.defer_id);
    const sorted = ids.slice().sort(lexCompare);
    for (let i = 0; i < ids.length; i++) {
        if (ids[i] !== sorted[i]) {
            return { ok: false, error: `records not sorted lexically by defer_id at index ${i}: ${ids[i]} before ${sorted[i]}` };
        }
    }
    return { ok: true };
}

// Apply the disposition matrix to one record. Returns:
//   { result: "allow", disclose: true, overrideAccepted?: true, why: "..." }
//   { result: "refuse", why: "..." }
// `releaseVersion` is the version being released (from --release-version).
// `overrideConfirmedVersion` is the operator-side wrapper confirmation
// (--override-confirmed-version). Override verification is split into two
// layers so the SAME evaluator serves both the pre-tag wrapper invocation
// and the post-tag CI recheck:
//   - Layer A (object validity): override object is present and well-formed,
//     override.release_version matches releaseVersion. Verifiable from the
//     committed manifest alone. Both the wrapper and CI verify this layer.
//   - Layer B (operator live intent): when --override-confirmed-version IS
//     supplied (wrapper mode), it MUST equal releaseVersion exactly. When it
//     is NOT supplied (CI defense-in-depth recheck), Layer A alone is
//     sufficient — the committed override object IS the attestation at that
//     point, and the wrapper already enforced Layer B before forwarding the
//     flag. The wrapper adds a post-evaluator gate so that an override
//     accepted by Layer A still refuses at tag time when the operator did not
//     supply the ceremony flags (--override-release-version +
//     --override-manifest-sha). This keeps wrapper enforcement whole without
//     weakening CI's verification role.
function applyDisposition(record, releaseVersion, overrideConfirmedVersion) {
    const rel = record.release_relevance;
    const disp = record.disposition;
    const meta = record.metadata_state;
    const override = record.override;
    if (rel === "yes") {
        if (disp === "block") {
            return { result: "refuse", why: "release_relevance=yes disposition=block: hard block; override cannot cure" };
        }
        if (disp === "disclose") {
            if (meta === "valid") {
                return { result: "allow", disclose: true, why: "release_relevance=yes disposition=disclose metadata_state=valid: disclosed" };
            }
            return { result: "refuse", why: `release_relevance=yes disposition=disclose requires metadata_state=valid; got ${meta}` };
        }
        if (disp === "override_required") {
            // Layer A (object validity) — both wrapper and CI verify.
            if (!override) {
                return { result: "refuse", why: "disposition=override_required but override object is absent" };
            }
            if (!releaseVersion) {
                return { result: "refuse", why: `override_required record requires --release-version; override declares ${override.release_version}` };
            }
            if (override.release_version !== releaseVersion) {
                return { result: "refuse", why: `override.release_version=${override.release_version} != release_version=${releaseVersion}` };
            }
            // Layer B (operator live intent) — wrapper mode only. When the
            // flag is supplied, it must match exactly. When absent (CI mode),
            // Layer A alone is sufficient; the wrapper's post-evaluator gate
            // enforces ceremony at tag time.
            if (overrideConfirmedVersion && overrideConfirmedVersion !== releaseVersion) {
                return { result: "refuse", why: `override-confirmed-version=${overrideConfirmedVersion} != release_version=${releaseVersion}` };
            }
            return {
                result: "allow", disclose: true, overrideAccepted: true,
                why: `override accepted for release ${releaseVersion} (approved_by=${override.approved_by})`,
            };
        }
        return { result: "refuse", why: `unhandled disposition for release_relevance=yes: ${disp}` };
    }
    if (rel === "no") {
        if (disp === "disclose") {
            return { result: "allow", disclose: true, why: `release_relevance=no disposition=disclose metadata_state=${meta}: disclosed as non-release-relevant` };
        }
        return { result: "refuse", why: `policy error: release_relevance=no is incompatible with disposition=${disp} (use disclose)` };
    }
    // rel === "unknown"
    return { result: "refuse", why: "release_relevance=unknown must be resolved to yes|no before release" };
}

// Emit the manifest-authority release envelope and exit. clear|disclose → 0;
// blocker → 1; evaluator-error → 2. Reuses emitReleaseResult (shared with
// legacy release mode) which already maps disclose → 0.

// MANIFEST-AUTHORITY release entrypoint. Strict schema-v1 + handshake + matrix.
function mainReleaseManifest(options) {
    const manifestPath = releaseManifestAbsPath();
    const releaseVersion = options.releaseVersion || null;
    const overrideConfirmedVersion = options.overrideConfirmedVersion || null;

    const envelope = {
        mode: "release",
        manifest_authority: true,
        manifest_path: manifestPath,
        manifest_sha: null,
        release_version: releaseVersion,
        override_confirmed_version: overrideConfirmedVersion,
        release_base: null,
        evaluated_commit: null,
        evaluated_tree: null,
        manifest_parent_commit: null,
        head_parent: null,
        head_parent_tree: null,
        reconciliation: null,
        records: [],
        disclosures: [],
        accepted_overrides: [],
        refusals: [],
        blocking_ids: [],
        disclose_ids: [],
        evaluator_error_ids: [],
        classification: "evaluator-error",
        error: null,
    };

    // 1. Read manifest bytes FROM THE COMMITTED HEAD BLOB (never the worktree).
    //
    // This is the manifest-authority contract: the bytes evaluated MUST equal
    // the bytes a fresh checkout (and CI) will see. Reading the worktree file
    // would let a dirty edit flip a committed `block` record to
    // `disclose+valid` while leaving the handshake SHAs intact — the wrapper's
    // subsequent HEAD^..HEAD path-list check would still pass (the manifest
    // path is unchanged), but the bytes evaluated would not be the bytes
    // committed at HEAD. Reading from `HEAD:<path>` makes that bypass
    // impossible: the evaluator and CI see the same bytes by construction.
    //
    // `manifestPath` is still computed (above) for the envelope's
    // `manifest_path` field, but the bytes themselves come from git, not fs.
    const raw = gitShowHeadBlob(RELEASE_MANIFEST_REL);
    if (raw === null) {
        envelope.error = `release manifest missing from HEAD: ${RELEASE_MANIFEST_REL} (must be committed, not just on worktree)`;
        emitReleaseResult(envelope);
        return;
    }

    // 2. Parse JSON.
    let obj;
    try {
        obj = JSON.parse(raw);
    } catch (e) {
        envelope.error = `release manifest malformed JSON: ${(e && e.message) || "parse error"}`;
        emitReleaseResult(envelope);
        return;
    }

    // 3. Manifest blob SHA FROM THE SAME COMMITTED BLOB (override binding +
    // echo). Computed via `git rev-parse HEAD:<path>` so it equals what CI
    // will also see — a dirty worktree cannot swap this SHA under the
    // override ceremony.
    envelope.manifest_sha = gitHeadBlobSha(RELEASE_MANIFEST_REL);
    if (!envelope.manifest_sha) {
        envelope.error = `release manifest unreadable from HEAD: ${RELEASE_MANIFEST_REL} (git rev-parse HEAD:<path> failed)`;
        emitReleaseResult(envelope);
        return;
    }

    // 4. Schema validation (v1; enums; sort; duplicates; per-record shape).
    const v = validateReleaseManifest(obj);
    if (!v.ok) {
        envelope.error = `manifest schema invalid: ${v.error}`;
        emitReleaseResult(envelope);
        return;
    }
    envelope.release_base = obj.release_base;
    envelope.evaluated_commit = obj.evaluated_commit;
    envelope.evaluated_tree = obj.evaluated_tree;
    envelope.manifest_parent_commit = obj.manifest_parent_commit;
    envelope.reconciliation = obj.reconciliation;
    envelope.records = obj.records;

    // 5. Freshness handshake (sacred — do not weaken).
    const headParent = gitHeadParent();
    const headParentTree = gitHeadParentTree();
    envelope.head_parent = headParent;
    envelope.head_parent_tree = headParentTree;
    if (!headParent || !headParentTree) {
        envelope.error = "handshake failed: HEAD^ does not exist (need at least 2 commits)";
        emitReleaseResult(envelope);
        return;
    }
    if (obj.evaluated_commit !== headParent) {
        envelope.error = `handshake failed: evaluated_commit=${obj.evaluated_commit} != HEAD^=${headParent}`;
        emitReleaseResult(envelope);
        return;
    }
    if (obj.manifest_parent_commit !== headParent) {
        envelope.error = `handshake failed: manifest_parent_commit=${obj.manifest_parent_commit} != HEAD^=${headParent}`;
        emitReleaseResult(envelope);
        return;
    }
    if (obj.evaluated_tree !== headParentTree) {
        envelope.error = `handshake failed: evaluated_tree=${obj.evaluated_tree} != tree(HEAD^)=${headParentTree}`;
        emitReleaseResult(envelope);
        return;
    }
    const diff = gitDiffHeadRange();
    if (diff === null) {
        envelope.error = "handshake failed: cannot compute HEAD^..HEAD diff";
        emitReleaseResult(envelope);
        return;
    }
    if (diff.length !== 1 || diff[0] !== RELEASE_MANIFEST_REL) {
        envelope.error = `handshake failed: HEAD^..HEAD must change only the manifest (${RELEASE_MANIFEST_REL}); got [${diff.join(", ")}]`;
        emitReleaseResult(envelope);
        return;
    }

    // 6. Release base validation.
    if (obj.release_base.kind === "tag") {
        // When --release-version is supplied (CI post-tag recheck), exclude
        // the just-cut release tag from the lookup so release_base is
        // validated against the PRIOR tag the manifest names. Pre-tag
        // wrapper invocations also forward --release-version, but the new
        // tag does not exist yet so the exclusion is a no-op there.
        const latest = gitLatestTag(releaseVersion);
        if (latest !== obj.release_base.value) {
            envelope.error = `release_base mismatch: manifest declares tag=${obj.release_base.value}; prior-tag lookup yields ${latest || "<none>"}`;
            emitReleaseResult(envelope);
            return;
        }
    }
    // kind=root: first release. Whole history is in scope; the manifest attests
    // relevance for that whole-history arc. No HEAD~32 fallback in manifest mode.

    // 7. Disposition matrix per record.
    const disclosures = [];
    const acceptedOverrides = [];
    const refusals = [];
    for (const record of obj.records) {
        const r = applyDisposition(record, releaseVersion, overrideConfirmedVersion);
        if (r.result === "allow" && r.disclose) {
            disclosures.push({
                defer_id: record.defer_id,
                release_relevance: record.release_relevance,
                disposition: record.disposition,
                metadata_state: record.metadata_state,
                summary: record.summary,
                reason: record.reason,
                source_ref: record.source_ref,
                override: record.override || null,
                why: r.why,
            });
            if (r.overrideAccepted) {
                const o = record.override;
                acceptedOverrides.push({
                    defer_id: record.defer_id,
                    release_version: o.release_version,
                    approved_by: o.approved_by,
                    approved_at: o.approved_at,
                    reason: o.reason,
                });
            }
        } else if (r.result === "refuse") {
            refusals.push({ defer_id: record.defer_id, why: r.why });
        }
    }
    envelope.disclosures = disclosures;
    envelope.accepted_overrides = acceptedOverrides;
    envelope.refusals = refusals;

    // 8. Aggregate classification.
    // Records-driven refusals → blocker (manifest is well-formed; release is
    // blocked pending resolution or override). Schema/handshake problems were
    // caught above and already returned as evaluator-error.
    if (refusals.length > 0) {
        envelope.classification = "blocker";
        envelope.blocking_ids = refusals.map((r) => r.defer_id).sort(lexCompare);
        envelope.disclose_ids = disclosures.map((d) => d.defer_id).sort(lexCompare);
        // Include each refusal's `why` so the operator can see exactly which
        // check failed (override confirmation missing, version mismatch, etc.)
        // without having to drill into refusals[].
        const detail = refusals
            .slice()
            .sort((a, b) => lexCompare(a.defer_id, b.defer_id))
            .map((r) => `${r.defer_id}: ${r.why}`)
            .join("; ");
        envelope.error = `${refusals.length} blocking release-defer record(s): ${detail}`;
    } else if (disclosures.length > 0) {
        envelope.classification = "disclose";
        envelope.disclose_ids = disclosures.map((d) => d.defer_id).sort(lexCompare);
        envelope.error = null;
    } else {
        envelope.classification = "clear";
        envelope.error = null;
    }

    emitReleaseResult(envelope);
}

// RELEASE-mode entrypoint. Routes between MANIFEST AUTHORITY (env active) and
// LEGACY (.local/ scan, byte-identical to the pre-manifest behavior) based on
// RELEASE_DEFER_MANIFEST_AUTHORITY. The split keeps the rollout staged: Phase
// 1 lands manifest support without making it mandatory; the activation step
// (operator-set env in the consuming release surfaces) flips the gate from advisory to enforced.
function mainRelease(options) {
    if (releaseManifestAuthorityActive()) {
        mainReleaseManifest(options);
        return;
    }
    mainReleaseLegacy(options);
}

// RELEASE-mode LEGACY entrypoint (reads .local/). Byte-identical to the
// pre-manifest release behavior; preserved for backward compatibility during
// the rollout. When RELEASE_DEFER_MANIFEST_AUTHORITY becomes the hardcoded
// default, this function and its .local/ primitives can be retired.
function mainReleaseLegacy(options) {
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
