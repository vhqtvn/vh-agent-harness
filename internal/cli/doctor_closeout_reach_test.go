package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Tests for doctor check #24 closeout-reach: the BOTH-WAYS ledger reachability
// reconciler. Coverage:
//   - unit (checkCloseoutReach direct):
//     dir-1 (unreachable committed SHA) -> WARN; reachable SHA -> no WARN.
//     dir-2 (unledgered branch commit) -> INFO; ledgered commit -> not listed.
//     combined (dir-1 + dir-2) -> WARN with BOTH in detail.
//     greenfield (no ledger / no committed entries / not a work tree) -> SKIP.
//     never FAIL (WARN/INFO/SKIP/PASS only — visibility, not refusal).
//   - integration (runDoctor output): a real doctor run surfaces the
//     closeout-reach section with the right tier.

// recReach builds a ledger record map for a committed closeout with the given
// post_commit_head (the schema the commit-gate appends at closeout time).
func recReach(post string) map[string]string {
	return map[string]string{
		"uuid": "t", "acquired_at": "", "head_at_acquire": "",
		"post_commit_head": post, "status": "committed", "branch": "main", "ts": "",
	}
}

// reachRepo creates a temp git repo configured for commits and returns (dir,
// commit). commit(msg) writes a unique file, commits it, and returns the new
// HEAD SHA. Tests drive commit() to synthesize reachable / orphaned / unledgered
// states, then seed the ledger with seedReachLedger.
func reachRepo(t *testing.T) (dir string, commit func(msg string) string) {
	t.Helper()
	dir = t.TempDir()
	mustGit := func(args ...string) {
		t.Helper()
		out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	mustGit("init", "-q")
	mustGit("config", "user.email", "test@example.com")
	mustGit("config", "user.name", "reach-test")
	commit = func(msg string) string {
		t.Helper()
		// Unique filename per call so each commit is a distinct tree.
		f := filepath.Join(dir, fmt.Sprintf("f-%d.txt", time.Now().UnixNano()))
		if err := os.WriteFile(f, []byte(msg), 0o644); err != nil {
			t.Fatalf("write %s: %v", f, err)
		}
		mustGit("add", f)
		mustGit("commit", "-q", "-m", msg)
		out, err := exec.Command("git", "-C", dir, "rev-parse", "HEAD").Output()
		if err != nil {
			t.Fatalf("rev-parse HEAD: %v", err)
		}
		return strings.TrimSpace(string(out))
	}
	return dir, commit
}

// seedReachLedger writes the given committed post_commit_head SHAs as JSON-lines
// into .git/commit-gate/closeouts.log (the path the commit-gate writes and the
// check reads). One ledger entry per SHA.
func seedReachLedger(t *testing.T, dir string, shas []string) {
	t.Helper()
	gateDir := filepath.Join(dir, ".git", "commit-gate")
	if err := os.MkdirAll(gateDir, 0o755); err != nil {
		t.Fatalf("mkdir gate: %v", err)
	}
	var buf strings.Builder
	for _, s := range shas {
		line, err := json.Marshal(recReach(s))
		if err != nil {
			t.Fatalf("marshal record: %v", err)
		}
		buf.Write(line)
		buf.WriteByte('\n')
	}
	if err := os.WriteFile(filepath.Join(gateDir, "closeouts.log"), []byte(buf.String()), 0o644); err != nil {
		t.Fatalf("write ledger: %v", err)
	}
}

// resetTo moves the current branch to sha (the operator git-reset escape hatch),
// orphaning any descendant commits. Tests use this to synthesize dir-1 orphans.
func resetTo(t *testing.T, dir, sha string) {
	t.Helper()
	if out, err := exec.Command("git", "-C", dir, "reset", "--hard", sha).CombinedOutput(); err != nil {
		t.Fatalf("git reset --hard %s: %v\n%s", sha, err, out)
	}
}

// TestCheckCloseoutReach_Unit drives the check directly against synthetic repos.
func TestCheckCloseoutReach_Unit(t *testing.T) {
	// dir-1: ledger claims a committed SHA that is NOT reachable from any branch
	// (orphaned via git reset). Must WARN naming the orphan; the reachable case
	// must NOT WARN.
	t.Run("unreachable_committed_WARN", func(t *testing.T) {
		dir, commit := reachRepo(t)
		c0 := commit("root")
		orphan := commit("second") // will be reset away
		seedReachLedger(t, dir, []string{orphan})
		resetTo(t, dir, c0) // orphan dangling, not on any branch
		r := checkCloseoutReach(dir)
		if r.tier != tierWarn {
			t.Fatalf("dir-1 orphan: tier = %q, want WARN; detail=%q", r.tier, r.detail)
		}
		if !strings.Contains(r.detail, closeoutReachShortID(orphan)) {
			t.Errorf("dir-1 WARN detail should name the orphan short SHA %s; got %q", closeoutReachShortID(orphan), r.detail)
		}
		if !strings.Contains(r.detail, "orphaned") && !strings.Contains(r.detail, "NOT reachable") {
			t.Errorf("dir-1 WARN detail should explain reachability; got %q", r.detail)
		}
	})

	t.Run("reachable_committed_no_WARN", func(t *testing.T) {
		dir, commit := reachRepo(t)
		commit("root")
		head := commit("second") // on the branch tip -> reachable
		seedReachLedger(t, dir, []string{head})
		r := checkCloseoutReach(dir)
		if r.tier == tierWarn {
			t.Fatalf("reachable committed SHA must NOT WARN; got WARN: %q", r.detail)
		}
	})

	// Case-domain: a hand-edited ledger line whose post_commit_head is
	// UPPERCASE (isValidFullSHA is case-insensitive) but the commit IS
	// reachable. The parser must lowercase-normalize at the parse site so
	// the uppercase ledger key reconciles against the lowercase reachable
	// map → no false dir-1 WARN. This is the exact case the fix targets
	// (mirrors reachable_committed_no_WARN but with a hand-edited uppercase
	// SHA); without the normalize, head would be keyed uppercase in ledgered
	// and miss the lowercase reachable[sha] map key → false orphan WARN.
	t.Run("uppercase_sha_reachable_no_WARN", func(t *testing.T) {
		dir, commit := reachRepo(t)
		commit("root")
		head := commit("second")                                 // reachable branch tip (lowercase from git)
		seedReachLedger(t, dir, []string{strings.ToUpper(head)}) // hand-edited uppercase post_commit_head
		r := checkCloseoutReach(dir)
		if r.tier == tierWarn {
			t.Fatalf("uppercase reachable committed SHA must NOT WARN (case-normalized at parse time); got WARN: %q", r.detail)
		}
		if r.tier != tierPass {
			t.Fatalf("uppercase reachable committed SHA: tier = %q, want PASS (reachable, anchor at HEAD, no unledgered); detail=%q", r.tier, r.detail)
		}
	})

	// dir-2: a reachable branch commit with NO ledger entry. Must surface INFO
	// naming the unledgered commit; a ledgered commit must NOT be listed.
	t.Run("unledgered_branch_commit_INFO", func(t *testing.T) {
		dir, commit := reachRepo(t)
		anchor := commit("root")
		seedReachLedger(t, dir, []string{anchor}) // last ledgered point
		unledgered := commit("second")            // reachable, NOT ledgered
		r := checkCloseoutReach(dir)
		if r.tier != tierInfo {
			t.Fatalf("dir-2 unledgered: tier = %q, want INFO; detail=%q", r.tier, r.detail)
		}
		if !strings.Contains(r.detail, closeoutReachShortID(unledgered)) {
			t.Errorf("dir-2 INFO detail should name the unledgered short SHA %s; got %q", closeoutReachShortID(unledgered), r.detail)
		}
	})

	t.Run("all_ledgered_no_INFO", func(t *testing.T) {
		dir, commit := reachRepo(t)
		commit("root")
		c1 := commit("second")
		seedReachLedger(t, dir, []string{c1}) // HEAD=c1 ledgered + reachable; anchor=c1
		r := checkCloseoutReach(dir)
		if r.tier == tierInfo {
			t.Fatalf("fully-ledgered branch must NOT surface INFO; got INFO: %q", r.detail)
		}
		if r.tier != tierPass {
			t.Fatalf("fully-ledgered reachable branch: tier = %q, want PASS; detail=%q", r.tier, r.detail)
		}
	})

	// Combined: an orphaned committed SHA (dir-1 WARN) AND an unledgered branch
	// commit after a reachable anchor (dir-2). WARN must dominate the tier and
	// the detail must carry BOTH findings.
	t.Run("both_dir1_WARN_and_dir2_INFO", func(t *testing.T) {
		dir, commit := reachRepo(t)
		commit("root")
		anchor := commit("ledgered-reachable") // will remain reachable (dir-2 anchor)
		orphan := commit("ledgered-then-orphaned")
		seedReachLedger(t, dir, []string{anchor, orphan})
		resetTo(t, dir, anchor)       // orphan dangling; branch back at anchor
		unledgered := commit("third") // reachable child of anchor, NOT ledgered
		r := checkCloseoutReach(dir)
		if r.tier != tierWarn {
			t.Fatalf("combined: tier = %q, want WARN (dir-1 dominates); detail=%q", r.tier, r.detail)
		}
		if !strings.Contains(r.detail, closeoutReachShortID(orphan)) {
			t.Errorf("combined WARN detail must name the orphan %s; got %q", closeoutReachShortID(orphan), r.detail)
		}
		if !strings.Contains(r.detail, closeoutReachShortID(unledgered)) {
			t.Errorf("combined WARN detail must ALSO name the unledgered %s (dir-2); got %q", closeoutReachShortID(unledgered), r.detail)
		}
	})

	// Greenfield: no ledger at all.
	t.Run("no_ledger_SKIP", func(t *testing.T) {
		dir, _ := reachRepo(t)
		commit := exec.Command("git", "-C", dir, "commit", "-q", "-m", "x", "--allow-empty")
		if out, err := commit.CombinedOutput(); err != nil {
			t.Fatalf("empty commit: %v\n%s", err, out)
		}
		r := checkCloseoutReach(dir)
		if r.tier != tierSkip {
			t.Errorf("no-ledger: tier = %q, want SKIP; detail=%q", r.tier, r.detail)
		}
	})

	// Greenfield: ledger exists but has NO committed entries (only failures).
	t.Run("no_committed_entries_SKIP", func(t *testing.T) {
		dir, _ := reachRepo(t)
		head := exec.Command("git", "-C", dir, "commit", "-q", "-m", "x", "--allow-empty")
		if out, err := head.CombinedOutput(); err != nil {
			t.Fatalf("empty commit: %v\n%s", err, out)
		}
		gateDir := filepath.Join(dir, ".git", "commit-gate")
		if err := os.MkdirAll(gateDir, 0o755); err != nil {
			t.Fatalf("mkdir gate: %v", err)
		}
		// A could_not_land entry (not committed) — must not trigger reconcile.
		rec, _ := json.Marshal(map[string]string{
			"post_commit_head": "deadbeef", "status": "could_not_land", "branch": "main",
		})
		if err := os.WriteFile(filepath.Join(gateDir, "closeouts.log"), append(rec, '\n'), 0o644); err != nil {
			t.Fatalf("write ledger: %v", err)
		}
		r := checkCloseoutReach(dir)
		if r.tier != tierSkip {
			t.Errorf("no committed entries: tier = %q, want SKIP; detail=%q", r.tier, r.detail)
		}
	})

	// Not a git work tree.
	t.Run("not_a_work_tree_SKIP", func(t *testing.T) {
		dir := t.TempDir()
		r := checkCloseoutReach(dir)
		if r.tier != tierSkip {
			t.Errorf("not-a-work-tree: tier = %q, want SKIP; detail=%q", r.tier, r.detail)
		}
	})

	// Malformed ledger lines are skipped (append-only best-effort).
	t.Run("malformed_lines_skipped", func(t *testing.T) {
		dir, commit := reachRepo(t)
		anchor := commit("root")
		commit("second") // unledgered
		gateDir := filepath.Join(dir, ".git", "commit-gate")
		if err := os.MkdirAll(gateDir, 0o755); err != nil {
			t.Fatalf("mkdir gate: %v", err)
		}
		good, _ := json.Marshal(recReach(anchor))
		// garbage line before the good one must not crash the check.
		content := "{not json\n" + string(good) + "\n"
		if err := os.WriteFile(filepath.Join(gateDir, "closeouts.log"), []byte(content), 0o644); err != nil {
			t.Fatalf("write ledger: %v", err)
		}
		r := checkCloseoutReach(dir)
		// anchor reachable + one unledgered commit -> INFO (the good line parsed).
		if r.tier != tierInfo {
			t.Fatalf("malformed-line skip: tier = %q, want INFO (good line parsed, 1 unledgered); detail=%q", r.tier, r.detail)
		}
	})

	// A JSON-valid committed record whose post_commit_head is a malformed SHA
	// (the literal "not-a-sha") is treated as a malformed line and SKIPPED —
	// never WARN'd as an orphan. The ledger has no valid committed entries,
	// so the check fails open to SKIP.
	t.Run("malformed_sha_only_SKIP", func(t *testing.T) {
		dir, _ := reachRepo(t)
		// An empty commit so HEAD resolves (the repo is otherwise unused: the
		// malformed-SHA record is skipped before any reachability work).
		empty := exec.Command("git", "-C", dir, "commit", "-q", "-m", "x", "--allow-empty")
		if out, err := empty.CombinedOutput(); err != nil {
			t.Fatalf("empty commit: %v\n%s", err, out)
		}
		seedReachLedger(t, dir, []string{"not-a-sha"})
		r := checkCloseoutReach(dir)
		if r.tier != tierSkip {
			t.Errorf("malformed-sha-only: tier = %q, want SKIP (malformed SHA skipped, no valid committed entries); detail=%q", r.tier, r.detail)
		}
		if strings.Contains(r.detail, "not-a-sha") {
			t.Errorf("malformed-sha-only: malformed SHA must not appear in detail; got %q", r.detail)
		}
	})

	// An invalid-SHA committed record alongside a VALID reachable committed
	// entry: the valid entry is reconciled (reachable → no orphan WARN), the
	// invalid one is ignored (not counted as an orphan). Result: clean PASS.
	t.Run("malformed_sha_ignored_alongside_valid", func(t *testing.T) {
		dir, commit := reachRepo(t)
		commit("root")
		head := commit("second") // reachable branch tip
		// Ledger: the valid reachable SHA + a malformed-SHA committed record.
		seedReachLedger(t, dir, []string{head, "not-a-sha"})
		r := checkCloseoutReach(dir)
		if r.tier == tierWarn {
			t.Fatalf("malformed-sha-alongside-valid: must NOT WARN (valid reachable, invalid ignored); got WARN: %q", r.detail)
		}
		if strings.Contains(r.detail, "not-a-sha") {
			t.Errorf("malformed-sha-alongside-valid: malformed SHA must not appear in detail; got %q", r.detail)
		}
		if r.tier != tierPass {
			t.Errorf("malformed-sha-alongside-valid: tier = %q, want PASS (valid entry reconciled, invalid ignored); detail=%q", r.tier, r.detail)
		}
	})

	// WARN-only authority: the check NEVER returns FAIL.
	t.Run("never_FAIL", func(t *testing.T) {
		dir, commit := reachRepo(t)
		c0 := commit("root")
		orphan := commit("second")
		seedReachLedger(t, dir, []string{orphan})
		resetTo(t, dir, c0)
		r := checkCloseoutReach(dir)
		if r.tier == tierFail {
			t.Fatalf("closeout-reach must NEVER FAIL (visibility, not refusal); got FAIL: %q", r.detail)
		}
	})
}

// doctorCloseoutReachOut runs the full doctor against a temp git repo with the
// given ledger and returns stdout (in-process runDoctor; exit error ignored so
// assertions focus on the closeout-reach section).
func doctorCloseoutReachOut(t *testing.T, dir string) string {
	t.Helper()
	var out string
	runWithCwd(t, dir, func() {
		doctorTargetFlag = dir
		defer func() { doctorTargetFlag = "" }()
		cmd, buf := newOutCmd()
		_ = runDoctor(cmd, []string{})
		out = buf.String()
	})
	return out
}

// TestDoctorCloseoutReach_Integration is the behavioral-closure crux: a REAL
// doctor run surfaces the closeout-reach section with the right tier for both
// directions.
func TestDoctorCloseoutReach_Integration(t *testing.T) {
	t.Run("dir1_WARN_section", func(t *testing.T) {
		dir, commit := reachRepo(t)
		c0 := commit("root")
		orphan := commit("second")
		seedReachLedger(t, dir, []string{orphan})
		resetTo(t, dir, c0)
		out := doctorCloseoutReachOut(t, dir)
		if !strings.Contains(out, "closeout-reach:") {
			t.Fatalf("doctor output must contain a closeout-reach section; got:\n%s", out)
		}
		if !strings.Contains(out, "closeout-reach WARN") {
			t.Errorf("dir-1 orphan must surface closeout-reach WARN; got:\n%s", out)
		}
	})

	t.Run("dir2_INFO_section", func(t *testing.T) {
		dir, commit := reachRepo(t)
		anchor := commit("root")
		seedReachLedger(t, dir, []string{anchor})
		commit("unledgered")
		out := doctorCloseoutReachOut(t, dir)
		if !strings.Contains(out, "closeout-reach:") {
			t.Fatalf("doctor output must contain a closeout-reach section; got:\n%s", out)
		}
		if !strings.Contains(out, "closeout-reach INFO") {
			t.Errorf("dir-2 unledgered must surface closeout-reach INFO; got:\n%s", out)
		}
	})

	t.Run("greenfield_SKIP_section", func(t *testing.T) {
		dir, _ := reachRepo(t)
		// no ledger -> SKIP, no WARN/INFO.
		out := doctorCloseoutReachOut(t, dir)
		if !strings.Contains(out, "closeout-reach:") {
			t.Fatalf("doctor output must contain a closeout-reach section (SKIP); got:\n%s", out)
		}
		if !strings.Contains(out, "closeout-reach SKIP") {
			t.Errorf("greenfield must surface closeout-reach SKIP; got:\n%s", out)
		}
		if strings.Contains(out, "closeout-reach WARN") || strings.Contains(out, "closeout-reach INFO") {
			t.Errorf("greenfield must NOT WARN/INFO; got:\n%s", out)
		}
	})
}
