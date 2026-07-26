package cli

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Tests for doctor check #19 head-progress (disposition §4.2 sub-item 2): the
// synchronous HEAD-staleness WARN that reads the durable commit-gate closeout
// ledger. Coverage:
//   - unit (checkHeadProgress direct): flatline -> WARN; greenfield (no ledger /
//     < N) -> SKIP; advances -> PASS; never FAIL (WARN-only authority, §5).
//   - integration (runDoctor output): a real doctor run against a synthetic
//     ledger surfaces the head-progress section with the right tier (crux #2).

func recHeadProgress(post, status string) map[string]string {
	return map[string]string{
		"uuid": "t", "acquired_at": "", "head_at_acquire": "",
		"post_commit_head": post, "status": status, "branch": "main", "ts": "",
	}
}

// writeLedgerRepo creates a temp git repo (so head-progress sees a work tree)
// and optionally seeds .git/commit-gate/closeouts.log. Returns the repo dir.
func writeLedgerRepo(t *testing.T, records []map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	gitIn := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	gitIn("init", "-q")
	if len(records) > 0 {
		gateDir := filepath.Join(dir, ".git", "commit-gate")
		if err := os.MkdirAll(gateDir, 0o755); err != nil {
			t.Fatalf("mkdir gate: %v", err)
		}
		var buf strings.Builder
		for _, r := range records {
			line, err := json.Marshal(r)
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
	return dir
}

// TestCheckHeadProgress_Unit covers the tier mapping directly (fast, targeted).
func TestCheckHeadProgress_Unit(t *testing.T) {
	const sha1 = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	const sha2 = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

	// Flatline: last 3 successful closeouts share the same post_commit_head.
	t.Run("flatline_WARN", func(t *testing.T) {
		dir := writeLedgerRepo(t, []map[string]string{
			recHeadProgress(sha1, "committed"),
			recHeadProgress(sha1, "committed"),
			recHeadProgress(sha1, "no_head_progress"),
		})
		r := checkHeadProgress(dir)
		if r.tier != tierWarn {
			t.Errorf("flatline: tier = %q, want WARN; detail=%q", r.tier, r.detail)
		}
		if !strings.Contains(r.detail, "not advanced") {
			t.Errorf("flatline WARN detail should mention HEAD not advancing; got %q", r.detail)
		}
	})

	// Greenfield: no ledger at all.
	t.Run("no_ledger_SKIP", func(t *testing.T) {
		dir := writeLedgerRepo(t, nil)
		r := checkHeadProgress(dir)
		if r.tier != tierSkip {
			t.Errorf("no-ledger: tier = %q, want SKIP; detail=%q", r.tier, r.detail)
		}
	})

	// Greenfield: fewer than N (3) successful closeouts.
	t.Run("under_threshold_SKIP", func(t *testing.T) {
		dir := writeLedgerRepo(t, []map[string]string{
			recHeadProgress(sha1, "committed"),
			recHeadProgress(sha1, "committed"),
		})
		r := checkHeadProgress(dir)
		if r.tier != tierSkip {
			t.Errorf("under-threshold: tier = %q, want SKIP; detail=%q", r.tier, r.detail)
		}
	})

	// Advances: last N successful closeouts have distinct post_commit_head.
	t.Run("advances_PASS", func(t *testing.T) {
		dir := writeLedgerRepo(t, []map[string]string{
			recHeadProgress(sha1, "committed"),
			recHeadProgress(sha2, "committed"),
			recHeadProgress(sha1, "committed"), // newest differs from prior → no flatline
		})
		r := checkHeadProgress(dir)
		if r.tier != tierPass {
			t.Errorf("advances: tier = %q, want PASS; detail=%q", r.tier, r.detail)
		}
	})

	// Non-successful closeouts (could_not_land) do NOT count toward the flatline.
	t.Run("could_not_land_excluded", func(t *testing.T) {
		dir := writeLedgerRepo(t, []map[string]string{
			recHeadProgress(sha1, "could_not_land"),
			recHeadProgress(sha1, "could_not_land"),
			recHeadProgress(sha1, "could_not_land"),
		})
		r := checkHeadProgress(dir)
		// 0 successful closeouts → under threshold → SKIP (a tangle of failures
		// is not a HEAD-flatline; the could_not_land status itself surfaces it).
		if r.tier != tierSkip {
			t.Errorf("could_not_land-only: tier = %q, want SKIP; detail=%q", r.tier, r.detail)
		}
	})

	// WARN-only authority (§5): the check NEVER returns FAIL.
	t.Run("never_FAIL", func(t *testing.T) {
		// Exercise every reachable branch and confirm none produces tierFail.
		for _, records := range [][]map[string]string{
			nil,
			{recHeadProgress(sha1, "committed")},
			{recHeadProgress(sha1, "committed"), recHeadProgress(sha1, "committed"), recHeadProgress(sha1, "committed")},
			{recHeadProgress(sha1, "committed"), recHeadProgress(sha2, "committed"), recHeadProgress(sha1, "committed")},
		} {
			dir := writeLedgerRepo(t, records)
			r := checkHeadProgress(dir)
			if r.tier == tierFail {
				t.Errorf("head-progress must NEVER FAIL (§5 WARN-only authority); got FAIL for records=%v: %q", records, r.detail)
			}
		}
	})
}

// doctorHeadProgressOut runs the full doctor against a temp git repo with the
// given ledger and returns stdout (in-process runDoctor; exit error ignored so
// assertions focus on the head-progress section, mirroring seamDoctorOut).
func doctorHeadProgressOut(t *testing.T, records []map[string]string) string {
	t.Helper()
	dir := writeLedgerRepo(t, records)
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

// TestDoctorHeadProgress_Integration is the behavioral-closure crux #2: a REAL
// doctor run surfaces the head-progress section with the right tier.
func TestDoctorHeadProgress_Integration(t *testing.T) {
	const sha1 = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

	// Flatline: N=3 identical post_commit_head → WARN section in doctor output.
	t.Run("flatline_WARN_section", func(t *testing.T) {
		out := doctorHeadProgressOut(t, []map[string]string{
			recHeadProgress(sha1, "committed"),
			recHeadProgress(sha1, "committed"),
			recHeadProgress(sha1, "committed"),
		})
		if !strings.Contains(out, "head-progress:") {
			t.Fatalf("doctor output must contain a head-progress section; got:\n%s", out)
		}
		if !strings.Contains(out, "head-progress WARN") {
			t.Errorf("flatline must surface head-progress WARN; got:\n%s", out)
		}
	})

	// Greenfield: no ledger → SKIP section, no WARN.
	t.Run("greenfield_SKIP_section", func(t *testing.T) {
		out := doctorHeadProgressOut(t, nil)
		if !strings.Contains(out, "head-progress:") {
			t.Fatalf("doctor output must contain a head-progress section (SKIP); got:\n%s", out)
		}
		if !strings.Contains(out, "head-progress SKIP") {
			t.Errorf("greenfield must surface head-progress SKIP; got:\n%s", out)
		}
		if strings.Contains(out, "head-progress WARN") {
			t.Errorf("greenfield must NOT WARN; got:\n%s", out)
		}
	})
}
