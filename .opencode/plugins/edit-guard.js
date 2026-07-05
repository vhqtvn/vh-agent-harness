// edit-guard.js — OpenCode plugin that curates the W1 single-writer-promotion
// denial message for direct edits to docs/planning/backlog.md.
//
// WHY: layer C (internal/permconfig) already DENIES worker edits to the
// canonical backlog via the permission table (BacklogPromoterDenyPath), but
// the denial opencode returns is a bare/unhelpful raw JSON rule-dump. This
// plugin intercepts the file-modification tools (edit/write/apply_patch) in
// tool.execute.before — BEFORE the permission gate — and throws a curated
// W1 message (rationale + transport commands + promoter-runbook pointer).
// This is the shell-guard pattern applied to file edits. Throwing in the hook
// aborts the tool-call Effect before the gate runs, so the bare gate message
// never emits.
//
// Agent-AGNOSTIC (O3 hint-only design — see shell-guard-core.js:~830):
// plugins do uniform hard-denies; agent-specific allow/deny stays in the
// per-agent permission table (layer C). The plugin does NOT identify the
// caller's agent type — it throws the SAME curated message for ALL agents
// hitting the protected path. layer C remains the defense-in-depth backstop
// (it enforces even if this plugin is disabled/removed); this plugin is the
// UX layer, layer C is the contract.
//
// Coverage: tool.execute.before fires for subagent tool calls too (empirically
// confirmed opencode 1.17.13). Three tools handled (branch on input.tool,
// mirroring shell-guard.js):
//   edit        -> output.args.filePath (string)
//   write       -> output.args.filePath (string)
//   apply_patch -> NO filePath arg; paths embedded in output.args.patchText
//                  (string) under *** Update File: <path> / *** Add File: <path>
//                  markers. Tool name is apply_patch, NOT patch (opencode issue
//                  #19941). We parse each marker path and check it.
//
// Single-file design: edit-guard's logic (match one path -> throw one message)
// is far simpler than shell-guard's bash-parsing engine, so there is no
// -core.js split. We reuse shell-guard-core.js's repoRoot() helper via the
// same import pattern shell-guard.js uses (cwd-robust path resolution).

import path from "node:path";
import { repoRoot } from "./shell-guard-core.js";

export const id = "edit-guard";

// The canonical task-status ledger path, repo-relative and exact (no glob).
// MUST match internal/permconfig/tables.go BacklogPromoterDenyPath. The edit
// tool passes path.relative(worktree, filePath), so the normal-case input is
// already repo-relative; this constant is the exact-match target after
// normalizeEditPath.
const PROTECTED_PATH = "docs/planning/backlog.md";

// Curated W1 denial message. MUST convey: (1) W1 single-writer-promotion
// rationale, (2) the sanctioned transport commands routing status intents to
// .local/coordinator/tasks/, (3) a pointer to the promoter runbook.
const W1_MESSAGE =
    "Blocked by edit-guard (W1 single-writer-promotion): agents must not " +
    "edit docs/planning/backlog.md directly — it is the canonical task-status " +
    "ledger, promoter-only. Route status intents to .local/coordinator/tasks/ " +
    "via /write-task, /task-update, /task-closeout, /task-review. The promoter " +
    "batch-promotes closeouts back into the backlog per cycle. See " +
    "docs/coordination/PROMOTER_RUNBOOK.md.";

// Normalize a target path to repo-root-relative form (no leading ./, no
// trailing /) for exact-match against PROTECTED_PATH. Mirrors the cwd-robust
// resolution in shell-guard-core.js: never uses process.cwd() (unreliable in
// the plugin server context — same rationale as repoRoot()). Absolute paths
// are resolved then made relative to repoRoot(); relative paths are normalized
// in place. Returns null for non-string / empty input so the caller can skip.
function normalizeEditPath(filePath, root) {
    if (!filePath || typeof filePath !== "string") return null;
    const base = root || repoRoot();
    const trimmed = filePath.trim();
    if (!trimmed) return null;
    const abs = path.isAbsolute(trimmed)
        ? path.resolve(trimmed)
        : path.resolve(base, trimmed);
    let rel = path.relative(base, abs);
    // Normalize separators to forward slashes (path.relative is platform-native;
    // the protected path uses /). On posix this is a no-op; on win32 it unifies.
    rel = rel.split(path.sep).join("/");
    // Strip a leading ./ (defensive — the edit tool already passes relative,
    // but be tolerant of any caller) and any trailing / (a directory can't
    // match a file path anyway, but keep the exact-match tidy).
    rel = rel.replace(/^\.\//, "").replace(/\/+$/, "");
    return rel;
}

// Extract target file paths from an apply_patch patchText. The standard opencode
// apply_patch markers are:
//   *** Add File: <path>
//   *** Delete File: <path>
//   *** Update File: <path>
// We match all three (Update/Add per the verified contract; Delete included
// defensively — deleting the ledger is also a status-tamper). Returns an array
// of raw path strings; the caller normalizes + exact-matches each. Non-matching
// lines (hunks, context, *** Begin Patch / *** End Patch) are ignored.
function extractApplyPatchPaths(patchText) {
    if (!patchText || typeof patchText !== "string") return [];
    // Defined locally (not module-level) to avoid any lastIndex shared-state
    // hazard across concurrent hook invocations. apply_patch is rare and the
    // regex is tiny, so per-call compilation is negligible.
    const re = /^\s*\*\*\* (?:Update|Add|Delete) File: (.+?)\s*$/gm;
    const out = [];
    let m;
    while ((m = re.exec(patchText)) !== null) {
        out.push(m[1].trim());
    }
    return out;
}

export const server = async () => {
    return {
        "tool.execute.before": async (input, output) => {
            // Branch on input.tool exactly like shell-guard.js. Only the three
            // file-modification tools are in scope; everything else falls through
            // (bare return -> opencode proceeds to its normal permission gate).
            if (input.tool === "edit" || input.tool === "write") {
                const rel = normalizeEditPath(output?.args?.filePath, repoRoot());
                if (rel === PROTECTED_PATH) {
                    throw new Error(W1_MESSAGE);
                }
                return;
            }

            if (input.tool === "apply_patch") {
                const paths = extractApplyPatchPaths(output?.args?.patchText);
                for (const raw of paths) {
                    const rel = normalizeEditPath(raw, repoRoot());
                    if (rel === PROTECTED_PATH) {
                        throw new Error(W1_MESSAGE);
                    }
                }
                return;
            }
        },
    };
};

export const EditGuardPlugin = server;

export default {
    id,
    server,
};
