package cli

// doctor_closeout_reach.go — the 24th doctor check: reconciles the durable
// commit-gate closeout ledger (.git/commit-gate/closeouts.log) against branch
// reachability in BOTH directions (the reconciler the operator git-reset /
// cherry-pick escape hatch leaves orphaned — see
// researches/decisions/2026-08-04-binding-regression-unification-audit.md
// Addendum):
//
//   - dir-1 (WARN): a ledger entry records status: committed against a
//     post_commit_head NOT reachable from any branch (orphaned). The object may
//     still exist (reflog/gc window), but no branch tip reaches it — the
//     operator git-reset escape hatch (legitimate; denied to agents by
//     git-mutation-bypass) moved the branch away after the gate recorded the
//     commit, so the ledger now references something that is no longer "landed".
//   - dir-2 (INFO/advisory): a commit reachable on the current branch has NO
//     ledger entry at all (work that landed without being recorded — e.g. a
//     cherry-pick through the escape hatch). Dir-2 is load-bearing: an unledgered
//     commit is indistinguishable from one that bypassed review — exactly the
//     visibility property the gate exists to provide.
//
// AUTHORITY: doctor INFORMs. dir-1 is WARN (NEVER FAIL); dir-2 is INFO
// (advisory; NEVER FAIL, NEVER WARN). The escape hatch is legitimate; the check
// provides VISIBILITY, not refusal. Greenfield (not a git work tree, no ledger,
// no committed entries) is SKIP, fail-open-safe — exactly like head-progress #19
// (consumers / fresh checkouts see nothing).
//
// REACHABILITY (not object existence) is the property that means "landed":
// `git show <sha>` succeeds for an orphaned / reflog-only commit, so object-
// existence cannot distinguish "committed and landed" from "committed and
// reverted/reset". This check asserts reachability via `git rev-list --branches`
// (the set of commits reachable from all branch tips — dir-1) and
// `git rev-list --max-count=<window> HEAD` (the current branch's recent commits,
// newest-first — dir-2 anchor walk). Both are now agent-allowed read-only
// plumbing (allowlist git_readonly; the prior permission gap — agents could run
// `git show`/`git rev-parse` object-existence but were denied `git merge-base`/
// `git rev-list` reachability — closed by 43f511e). `git merge-base
// --is-ancestor <sha> <branch>` is the canonical single-sha reachability
// verifier documented for behavioral-closure tokens; this check uses rev-list
// (set membership) because it reconciles the whole ledger in two git calls
// rather than one git call per SHA.

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// closeoutReachUnledgeredWindow bounds how far back dir-2 walks the current
// branch looking for the "last ledgered point" (the newest ledgered commit on
// HEAD's branch). The ledger is a 200-entry rolling window of recent closeouts,
// so the anchor is within recent history; a window of 2x the cap covers it with
// margin. If no ledgered commit is found within the window, dir-2 reports
// nothing (fail-open advisory — never a false positive). This bounds git output
// on very large repos so the check stays cheap.
const closeoutReachUnledgeredWindow = 500

// checkCloseoutReach is doctor check #24. See the file-level comment for the
// full reconcile contract. It is READ-ONLY: it never mutates the ledger and
// shells out only to read-only git plumbing.
func checkCloseoutReach(target string) checkResult {
	const name = "closeout-reach"
	if _, err := exec.LookPath("git"); err != nil {
		return checkResult{name: name, tier: tierSkip, detail: "git not on PATH"}
	}
	wt, err := exec.Command("git", "-C", target, "rev-parse", "--is-inside-work-tree").Output()
	if err != nil || strings.TrimSpace(string(wt)) != "true" {
		return checkResult{name: name, tier: tierSkip, detail: "not a git work tree"}
	}

	// Resolve the git dir exactly like head-progress #19 (handles worktrees /
	// submodules where .git is a file or under .git/worktrees/); the gate writes
	// its ledger at <gitdir>/commit-gate/.
	gitDirOut, err := exec.Command("git", "-C", target, "rev-parse", "--git-dir").Output()
	gitDir := strings.TrimSpace(string(gitDirOut))
	if err != nil || gitDir == "" {
		gitDir = ".git"
	}
	var ledgerPath string
	if filepath.IsAbs(gitDir) {
		ledgerPath = filepath.Join(gitDir, "commit-gate", "closeouts.log")
	} else {
		ledgerPath = filepath.Join(target, gitDir, "commit-gate", "closeouts.log")
	}
	data, err := os.ReadFile(ledgerPath)
	if err != nil {
		// Greenfield: no closeouts recorded yet (or ledger removed).
		return checkResult{name: name, tier: tierSkip,
			detail: "no closeout ledger yet (no committer closeouts recorded)"}
	}

	// Collect the post_commit_head of COMMITTED closeouts (the entries that
	// claim a successful land). could_not_land / rebased_refused / cas_conflict
	// entries record an unchanged HEAD that is structurally reachable and are
	// not "I landed a commit" claims, so they are out of scope for BOTH
	// directions (dir-1 is specifically about committed claims; dir-2's anchor
	// is a real landed commit). Dedup: multiple entries may share a
	// post_commit_head (rebased_refused shares the unchanged HEAD).
	ledgered := make(map[string]bool)
	committedEntries := 0
	for _, line := range strings.Split(string(data), "\n") {
		tl := strings.TrimSpace(line)
		if tl == "" {
			continue
		}
		var rec struct {
			PostCommitHead string `json:"post_commit_head"`
			Status         string `json:"status"`
		}
		if err := json.Unmarshal([]byte(tl), &rec); err != nil {
			continue // skip malformed line (append-only best-effort ledger)
		}
		// post_commit_head must be a valid full-length git object ID; a
		// malformed (empty / abbreviated / non-hex) value makes the record
		// malformed → skip (fail-open: a corrupt ledger line is never WARN'd
		// as an orphan — see isValidFullSHA).
		if rec.Status != "committed" || !isValidFullSHA(rec.PostCommitHead) {
			continue
		}
		committedEntries++
		ledgered[rec.PostCommitHead] = true
	}
	if len(ledgered) == 0 {
		// Greenfield: no committed closeouts recorded (only failures / empty).
		return checkResult{name: name, tier: tierSkip,
			detail: "no committed closeout entries in the ledger (nothing committed to reconcile)"}
	}

	// ---- dir-1: unreachable committed SHAs (WARN) ----
	// reachableFromBranches = every commit reachable from any branch tip.
	// A ledgered committed SHA not in this set is orphaned (recorded by the
	// gate but no longer on any branch). `git rev-list --branches` is the
	// precise "reachable from any branch" set (ancestors of all branch tips).
	reachable, ok := gitReachableFromBranches(target)
	if !ok {
		// git rev-list --branches failed unexpectedly (not the unborn-HEAD case,
		// which returns empty rather than erroring). Cannot reconcile safely.
		return checkResult{name: name, tier: tierSkip,
			detail: "could not enumerate branch-reachable commits (git rev-list --branches failed)"}
	}
	var unreachable []string
	for sha := range ledgered {
		if !reachable[sha] {
			unreachable = append(unreachable, sha)
		}
	}
	sort.Strings(unreachable)

	// ---- dir-2: unledgered branch commits (INFO/advisory) ----
	// Walk the current branch's recent commits newest-first; the FIRST ledgered
	// commit encountered is the "last ledgered point" (the anchor). Every commit
	// newer than it (already passed) that is not itself ledgered is an unledgered
	// branch commit — reachable work the gate never recorded. If no ledgered
	// commit is found within the window, dir-2 has no anchor and reports nothing
	// (fail-open; dir-1 owns the orphan case when ALL ledgered commits are off
	// the branch).
	recent, _ := gitRevListHead(target, closeoutReachUnledgeredWindow) // empty on error/unborn → dir-2 skipped
	var unledgered []string
	anchorFound := false
	for _, sha := range recent {
		if ledgered[sha] {
			anchorFound = true
			break
		}
		unledgered = append(unledgered, sha)
	}
	if !anchorFound {
		unledgered = nil
	}

	// ---- tier + detail ----
	switch {
	case len(unreachable) > 0:
		detail := fmt.Sprintf(
			"%s reference(s) a post_commit_head NOT reachable from any branch (orphaned): %s — recorded by the gate but no longer on a branch tip (operator git reset escape hatch, or gc'd); reconcile the ledger or re-land the commit(s)",
			pluralEntry(len(unreachable)), joinShortSHAs(unreachable))
		if len(unledgered) > 0 {
			detail += fmt.Sprintf(
				" | dir-2 advisory: %s reachable branch commit(s) since the last ledgered point with NO ledger entry: %s (operator cherry-pick/reset escape hatch; an unledgered commit is indistinguishable from one that bypassed review)",
				strconv.Itoa(len(unledgered)), joinShortSHAs(unledgered))
		}
		return checkResult{name: name, tier: tierWarn, detail: detail}
	case len(unledgered) > 0:
		return checkResult{name: name, tier: tierInfo,
			detail: fmt.Sprintf(
				"%s reachable branch commit(s) since the last ledgered point with NO closeout ledger entry: %s — work landed on the branch without being recorded by commit-gate (operator cherry-pick / reset escape hatch). Advisory: an unledgered commit is indistinguishable from one that bypassed review. The escape hatch is legitimate; this is visibility, not refusal.",
				strconv.Itoa(len(unledgered)), joinShortSHAs(unledgered))}
	default:
		return checkResult{name: name, tier: tierPass,
			detail: fmt.Sprintf(
				"ledger reconciles with branch reachability (%d committed entry/ies, all reachable from a branch; no unledgered branch commits in the last %d)",
				committedEntries, closeoutReachUnledgeredWindow)}
	}
}

// isValidFullSHA reports whether s is a full-length git object ID: 40 lowercase
// hex chars (SHA-1) or 64 lowercase hex chars (SHA-256). Git emits lowercase,
// but the check is case-insensitive (lenient on hand-edited / upserted ledger
// lines). A post_commit_head that fails this check (empty, an abbreviated hash,
// or a literal like "not-a-sha") is treated as a malformed record and SKIPPED
// by the ledger parser — never added to ledgered, so it cannot be misclassified
// as an orphaned commit (dir-1 WARN). This is the fail-open contract: malformed
// ledger lines are skipped, not WARN'd or FAIL'd.
func isValidFullSHA(s string) bool {
	switch len(s) {
	case 40, 64:
	default:
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

// gitReachableFromBranches returns the set of commit OIDs reachable from any
// branch tip (`git rev-list --branches`). Returns (set, true) on success, or
// (nil, false) if git errored (the unborn-HEAD case returns an empty set + true,
// not an error). Dedup is implicit (map).
func gitReachableFromBranches(target string) (map[string]bool, bool) {
	out, err := exec.Command("git", "-C", target, "rev-list", "--branches").Output()
	if err != nil {
		return nil, false
	}
	set := make(map[string]bool)
	for _, line := range strings.Split(string(out), "\n") {
		sha := strings.TrimSpace(line)
		if sha != "" {
			set[sha] = true
		}
	}
	return set, true
}

// gitRevListHead returns up to `limit` commit OIDs reachable from HEAD, newest-
// first (`git rev-list --max-count=<limit> HEAD`). Returns (nil, false) on error
// (e.g. unborn HEAD where HEAD does not resolve); the caller treats that as
// "no dir-2 anchor".
func gitRevListHead(target string, limit int) ([]string, bool) {
	out, err := exec.Command("git", "-C", target, "rev-list", "--max-count", strconv.Itoa(limit), "HEAD").Output()
	if err != nil {
		return nil, false
	}
	var list []string
	for _, line := range strings.Split(string(out), "\n") {
		sha := strings.TrimSpace(line)
		if sha != "" {
			list = append(list, sha)
		}
	}
	return list, true
}

// joinShortSHAs renders up to closeoutReachDisplayMax SHAs as 12-char short IDs,
// comma-separated, with a trailing "... and N more" when truncated. Keeps the
// doctor detail line self-routing without flooding it on a large discrepancy.
func joinShortSHAs(shas []string) string {
	const closeoutReachDisplayMax = 8
	if len(shas) == 0 {
		return "(none)"
	}
	parts := make([]string, 0, len(shas))
	for _, s := range shas {
		parts = append(parts, closeoutReachShortID(s))
	}
	if len(parts) <= closeoutReachDisplayMax {
		return strings.Join(parts, ", ")
	}
	head := parts[:closeoutReachDisplayMax]
	return strings.Join(head, ", ") + fmt.Sprintf(", ... and %d more", len(parts)-closeoutReachDisplayMax)
}

// pluralEntry returns "1 entry" or "N entries" for detail prose.
func pluralEntry(n int) string {
	if n == 1 {
		return "1 committed ledger entry"
	}
	return fmt.Sprintf("%d committed ledger entries", n)
}

// closeoutReachShortID renders the first 12 chars of a git object id (mirrors
// headProgressShortID's compact detail rendering; kept local so the reach check
// is self-contained).
func closeoutReachShortID(s string) string {
	if len(s) > 12 {
		return s[:12]
	}
	return s
}
