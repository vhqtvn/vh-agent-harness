// backlog-reminder.js — OpenCode plugin that emits a NON-BLOCKING reminder when
// any agent edits docs/planning/backlog.md (the shared task-status ledger).
//
// WHY (the layer shift from the old edit-guard.js): the previous W1 layer
// DENIED worker edits to the backlog at the permission table AND threw a
// curated block from this plugin. That was the WRONG LAYER. The hybrid
// split-commit model says: agents edit the backlog FREELY; conflict
// discipline is enforced at the COMMIT layer (commit backlog separately from
// code; on cas_conflict re-read + re-apply + retry; never blind-revert). So
// this plugin no longer blocks — it NUDGES, pointing the agent at the
// `backlog` skill and the top disciplines, exactly once per session per path.
//
// DESIGN (O3 hint-only — see shell-guard.js:121-124 and shell-guard-core.js):
//   - tool.execute.before fires for subagent tool calls too (empirically
//     confirmed opencode 1.17.13). Three edit tools handled (branch on
//     input.tool, mirroring the old edit-guard + shell-guard):
//       edit        -> output.args.filePath (string)
//       write       -> output.args.filePath (string)
//       apply_patch -> paths embedded in output.args.patchText under
//                      *** Update|Add|Delete File: <path> markers
//   - bash is ALSO handled, but with a RAW SUBSTRING match on
//     output.args.command — NOT the shell-guard AST parser. The parser
//     (shell-guard-core.js:147-153) strips redirection nodes, so a redirected
//     write to the backlog (`... > docs/planning/backlog.md`) would slip past
//     an AST check. A substring match `/docs\/planning\/backlog\.md/` catches
//     the literal path regardless of how the command is shaped. This is
//     correct for a NON-BLOCKING nudge (a substring false-positive only
//     produces an extra reminder line, never a wrong block).
//   - NON-BLOCKING: console.error(REMINDER_MESSAGE) + bare return. NEVER
//     throw. Throwing is the block path; this plugin is hint-only. The
//     permission table (layer C) is the contract; this plugin is the UX layer.
//   - Once-per-session-per-path: an in-memory Set keyed by sessionID + path
//     suppresses repeat reminders within one process. opencode loads plugins
//     at process start, so "session" here means "process lifetime".
//
// The PROTECTED_PATH constant MUST stay in sync with internal/permconfig/
// tables.go BacklogLedgerPath (the cross-constant test
// TestEditGuardProtectedPathMatchesPermconfigConstant enforces this). Drift
// degrades UX (a reminder that fires on the wrong path, or fails to fire),
// not correctness — the permission table is still the contract.

import path from "node:path";
import { repoRoot } from "./shell-guard-core.js";

export const id = "backlog-reminder";

// The canonical task-status ledger path, repo-relative and exact (no glob).
// MUST match internal/permconfig/tables.go BacklogLedgerPath
// (== "docs/planning/backlog.md"). The edit tool passes
// path.relative(worktree, filePath), so the normal-case input is already
// repo-relative; this constant is the exact-match target after
// normalizeEditPath.
const PROTECTED_PATH = "docs/planning/backlog.md";

// Non-blocking reminder message. Conveys: (1) what the file is, (2) the
// `backlog` skill is the right thing to load first, (3) the top disciplines
// (re-read, own rows, separate commit), (4) DEFER routing to holding, (5)
// the cas_conflict anti-pattern (do NOT revert), (6) this is not a block.
// The {{COORDINATOR_DIR}} token is rendered by the harness on `update` to
// the real coordinator dir name; at runtime the literal here is the rendered
// value.
const REMINDER_MESSAGE =
    "[backlog] You're editing docs/planning/backlog.md — the shared " +
    "task-status ledger. Load the `backlog` skill first: re-read from disk, " +
    "edit only your rows, and commit backlog SEPARATELY from code so a " +
    "concurrent edit can't block your code commit. DEFER findings → " +
    ".local/{{COORDINATOR_DIR}}/tasks/ (not this file). On cas_conflict, " +
    "re-read + retry (do NOT revert). Edit proceeding (not blocked).";

// In-process dedup: fire once per (sessionID + path) per process lifetime.
// opencode loads plugins at process start, so a process restart resets this.
const fired = new Set();
function shouldFire(sessionID, targetPath) {
    const key = `${sessionID || "no-session"}::${targetPath}`;
    if (fired.has(key)) return false;
    fired.add(key);
    return true;
}

// Normalize a target path to repo-root-relative form (no leading ./, no
// trailing /) for exact-match against PROTECTED_PATH. Mirrors the cwd-robust
// resolution in shell-guard-core.js: never uses process.cwd() (unreliable in
// the plugin server context — same rationale as repoRoot()). Absolute paths
// are resolved then made relative to repoRoot(); relative paths are
// normalized in place. Returns null for non-string / empty input so the
// caller can skip.
function normalizeEditPath(filePath, root) {
    if (!filePath || typeof filePath !== "string") return null;
    const base = root || repoRoot();
    const trimmed = filePath.trim();
    if (!trimmed) return null;
    const abs = path.isAbsolute(trimmed)
        ? path.resolve(trimmed)
        : path.resolve(base, trimmed);
    let rel = path.relative(base, abs);
    // Normalize separators to forward slashes (path.relative is platform-
    // native; the protected path uses /). On posix this is a no-op; on win32
    // it unifies.
    rel = rel.split(path.sep).join("/");
    // Strip a leading ./ (defensive — the edit tool already passes relative,
    // but be tolerant of any caller) and any trailing / (a directory can't
    // match a file path anyway, but keep the exact-match tidy).
    rel = rel.replace(/^\.\//, "").replace(/\/+$/, "");
    return rel;
}

// Extract target file paths from an apply_patch patchText. The standard
// opencode apply_patch markers are:
//   *** Add File: <path>
//   *** Delete File: <path>
//   *** Update File: <path>
// We match all three (Update/Add per the verified contract; Delete included
// defensively — deleting the ledger is also a status-tamper). Returns an
// array of raw path strings; the caller normalizes + exact-matches each.
// Defined locally (not module-level) to avoid any lastIndex shared-state
// hazard across concurrent hook invocations — apply_patch is rare and the
// regex is tiny, so per-call compilation is negligible.
function extractApplyPatchPaths(patchText) {
    if (!patchText || typeof patchText !== "string") return [];
    const re = /^\s*\*\*\* (?:Update|Add|Delete) File: (.+?)\s*$/gm;
    const out = [];
    let m;
    while ((m = re.exec(patchText)) !== null) {
        out.push(m[1].trim());
    }
    return out;
}

// Emit the reminder (once per session per path). Centralizes the dedup check
// and the console.error so every tool branch behaves identically.
function remind(sessionID) {
    if (shouldFire(sessionID, PROTECTED_PATH)) {
        console.error(REMINDER_MESSAGE);
    }
}

export const server = async () => {
    return {
        "tool.execute.before": async (input, output) => {
            const sessionID =
                (input && input.sessionID) ||
                (output && output.sessionID) ||
                "no-session";

            // edit / write: filePath arg.
            if (input.tool === "edit" || input.tool === "write") {
                const rel = normalizeEditPath(output?.args?.filePath, repoRoot());
                if (rel === PROTECTED_PATH) {
                    remind(sessionID);
                }
                return; // passthrough — NEVER throw, NEVER block.
            }

            // apply_patch: parse markers from patchText.
            if (input.tool === "apply_patch") {
                const paths = extractApplyPatchPaths(output?.args?.patchText);
                for (const raw of paths) {
                    const rel = normalizeEditPath(raw, repoRoot());
                    if (rel === PROTECTED_PATH) {
                        remind(sessionID);
                        break; // one reminder per call is enough
                    }
                }
                return; // passthrough — NEVER throw, NEVER block.
            }

            // bash: RAW SUBSTRING on the command. NOT the AST parser — the
            // parser strips redirection nodes (shell-guard-core.js:147-153),
            // so a redirected write would slip past an AST check. A substring
            // match catches the literal path regardless of command shape.
            // Correct for a non-blocking nudge; a false-positive only adds a
            // reminder line, never a wrong block.
            if (input.tool === "bash") {
                const command = output?.args?.command;
                if (typeof command === "string" && /docs\/planning\/backlog\.md/.test(command)) {
                    remind(sessionID);
                }
                return; // passthrough — NEVER throw, NEVER block.
            }

            // Any other tool: bare return (passthrough to the normal gate).
        },
    };
};

export const BacklogReminderPlugin = server;

export default {
    id,
    server,
};
