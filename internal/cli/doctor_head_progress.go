package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// defaultStaleHeadThreshold is the N for the head-progress flatline WARN: that
// many consecutive SUCCESSFUL closeouts (status committed / no_head_progress)
// with identical post_commit_head values means HEAD has not advanced across N
// closeouts — the canary for the ~6h commit-freeze symptom (Pattern 4 same-file
// tangle). Env override: COMMIT_GATE_STALE_HEAD_THRESHOLD (min 2).
const defaultStaleHeadThreshold = 3

// checkHeadProgress is doctor check #19 (disposition §4.2 sub-item 2): a
// synchronous HEAD-staleness WARN that reads the durable closeout ledger
// (.git/commit-gate/closeouts.log) the commit-gate appends to on each closeout
// (sub-item 1, landed in a4afe6e8). It WARNs when the last N successful
// closeouts all recorded the SAME post_commit_head (a flatline — closeouts are
// "succeeding" but HEAD is not advancing).
//
// Authority (§5 L284): the doctor INFORMS via WARN. It NEVER changes the exit
// code and never gives the coordinator authority (gate/doctor act; coordinator
// reads). Greenfield — not a git work tree, ledger missing/unreadable, or fewer
// than N successful closeouts recorded — is SKIP, fail-open-safe, exactly like
// dev-stale-embed #15's not-applicable branch (consumers/fresh checkouts see
// nothing). Malformed ledger lines are skipped (the ledger is append-only
// best-effort; a single corrupt line must not crash doctor).
func checkHeadProgress(target string) checkResult {
	const name = "head-progress"
	if _, err := exec.LookPath("git"); err != nil {
		return checkResult{name: name, tier: tierSkip, detail: "git not on PATH"}
	}
	wt, err := exec.Command("git", "-C", target, "rev-parse", "--is-inside-work-tree").Output()
	if err != nil || strings.TrimSpace(string(wt)) != "true" {
		return checkResult{name: name, tier: tierSkip, detail: "not a git work tree"}
	}

	// Resolve the git dir (handles worktrees/submodules where .git is a file or
	// under .git/worktrees/); the gate writes its ledger at <gitdir>/commit-gate/.
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
		// Greenfield: no closeouts recorded yet (or ledger removed). Consumers and
		// fresh checkouts see SKIP — never WARN for a missing ledger.
		return checkResult{name: name, tier: tierSkip,
			detail: "no closeout ledger yet (no committer closeouts recorded)"}
	}

	threshold := staleHeadThreshold()
	// Parse JSON-lines; collect the post_commit_head of SUCCESSFUL closeouts
	// (status committed / no_head_progress). Only successful closeouts count
	// toward the flatline (a could_not_land / cas_conflict did not move HEAD).
	var heads []string
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
		if rec.Status != "committed" && rec.Status != "no_head_progress" {
			continue
		}
		if rec.PostCommitHead == "" {
			continue
		}
		heads = append(heads, rec.PostCommitHead)
	}
	if len(heads) < threshold {
		// Greenfield: fewer than N successful closeouts recorded.
		return checkResult{name: name, tier: tierSkip,
			detail: fmt.Sprintf("%d successful closeout(s) recorded (< threshold %d); not enough history to detect a flatline", len(heads), threshold)}
	}

	// Take the last N; flatline iff they are ALL identical.
	tail := heads[len(heads)-threshold:]
	first := tail[0]
	flat := true
	for _, h := range tail[1:] {
		if h != first {
			flat = false
			break
		}
	}
	if flat {
		return checkResult{name: name, tier: tierWarn,
			detail: fmt.Sprintf(
				"HEAD has not advanced across the last %d successful committer closeouts (post_commit_head flatlined at %s) — commits may be recording as successful without landing (Pattern 4 same-file tangle); inspect the commit-gate closeout ledger and recent commits",
				threshold, headProgressShortID(first))}
	}
	return checkResult{name: name, tier: tierPass,
		detail: fmt.Sprintf("HEAD advanced across the last %d successful closeout(s) (no flatline)", threshold)}
}

// staleHeadThreshold reads COMMIT_GATE_STALE_HEAD_THRESHOLD (min 2); falls back
// to defaultStaleHeadThreshold.
func staleHeadThreshold() int {
	if v := os.Getenv("COMMIT_GATE_STALE_HEAD_THRESHOLD"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 2 {
			return n
		}
	}
	return defaultStaleHeadThreshold
}

// headProgressShortID renders the first 12 chars of a git object id for compact
// detail output.
func headProgressShortID(s string) string {
	if len(s) > 12 {
		return s[:12]
	}
	return s
}
