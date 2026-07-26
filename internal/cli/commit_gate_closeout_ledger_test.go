package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// TestCommitGate_CloseoutLedger is the Slice A regression guard for the durable
// closeout ledger (disposition §4.2 sub-item 1: "committer closeout records
// post-commit HEAD alongside the existing per-session head_at_acquire"). It
// black-boxes the RENDERED .opencode/scripts/commit-gate.sh (NOT the template)
// inside a fully-isolated scratch git repo, mirroring TestCommitGate_MessageCleanup's
// harness (isolated .opencode/scripts/ copy, minimal valid opencode.jsonc + one
// agents/*.md, git init + initial commit, drive the real gate, assert on FS).
//
// Coverage (each subtest is a real behavioral outcome, not mechanism-assertion):
//   - records_post_commit_head:        acquire→commit appends a record to
//     .git/commit-gate/closeouts.log carrying the full schema; post_commit_head
//     equals the actual branch HEAD after the commit landed (outcome-observed);
//     status == "committed"; head_at_acquire == the acquire-time HEAD.
//   - ledger_survives_session_meta_cleanup: after commit, the transient
//     meta-${uuid} is reclaimed BUT closeouts.log remains (the durability
//     property that makes doctor's #19 N-flatline read possible post-cleanup).
//   - gc_count_cap_trims:              _gate_gc_sweep trims the append-only
//     ledger to the tail (most recent N) when the line count exceeds
//     COMMIT_GATE_CLOSEOUT_LOG_MAX, keeping the records doctor actually reads.
func TestCommitGate_CloseoutLedger(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not on PATH")
	}
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not on PATH")
	}

	repoRoot := filepath.Join("..", "..")
	srcScripts := filepath.Join(repoRoot, ".opencode", "scripts")
	for _, f := range []string{"commit-gate.sh", "readonly-scripts.sh", "validate-opencode-config.py"} {
		if _, err := os.Stat(filepath.Join(srcScripts, f)); err != nil {
			t.Skipf("rendered script %s unavailable: %v (run `vh-agent-harness update` first)", f, err)
		}
	}

	setupScratchRepo := func(t *testing.T) (dir, dstScripts string) {
		t.Helper()
		dir = t.TempDir()
		dstScripts = filepath.Join(dir, ".opencode", "scripts")
		if err := os.MkdirAll(dstScripts, 0o755); err != nil {
			t.Fatalf("mkdir scripts: %v", err)
		}
		for _, f := range []string{"commit-gate.sh", "readonly-scripts.sh", "validate-opencode-config.py"} {
			data, err := os.ReadFile(filepath.Join(srcScripts, f))
			if err != nil {
				t.Fatalf("read %s: %v", f, err)
			}
			if err := os.WriteFile(filepath.Join(dstScripts, f), data, 0o755); err != nil {
				t.Fatalf("write %s: %v", f, err)
			}
		}
		if err := os.WriteFile(filepath.Join(dir, "opencode.jsonc"),
			[]byte(`{ "$schema": "https://opencode.ai/config.json", "agent": { "build": { "description": "test" } } }`),
			0o644); err != nil {
			t.Fatalf("write opencode.jsonc: %v", err)
		}
		agentsDir := filepath.Join(dir, ".opencode", "agents")
		if err := os.MkdirAll(agentsDir, 0o755); err != nil {
			t.Fatalf("mkdir agents: %v", err)
		}
		if err := os.WriteFile(filepath.Join(agentsDir, "build.md"),
			[]byte("---\ndescription: test\nmode: primary\n---\n# build\n"), 0o644); err != nil {
			t.Fatalf("write build.md: %v", err)
		}
		gitIn := func(args ...string) {
			t.Helper()
			cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("git %v: %v\n%s", args, err, out)
			}
		}
		gitIn("init", "-q")
		// Ensure the default branch is "main" for a stable branch assertion.
		gitIn("symbolic-ref", "HEAD", "refs/heads/main")
		gitIn("config", "user.email", "t@t")
		gitIn("config", "user.name", "t")
		gitIn("config", "commit.gpgsign", "false")
		return dir, dstScripts
	}

	genUUID := func(t *testing.T, dstScripts string) string {
		t.Helper()
		out, err := exec.Command("bash", filepath.Join(dstScripts, "readonly-scripts.sh"), "gen-uuid").Output()
		if err != nil {
			t.Fatalf("gen-uuid: %v", err)
		}
		return strings.TrimSpace(string(out))
	}

	writeAgentMsg := func(t *testing.T, dir, uuid string) string {
		t.Helper()
		rel := filepath.ToSlash(filepath.Join("tmp", "commit-gate-message", "msg-"+uuid))
		full := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir scratch: %v", err)
		}
		if err := os.WriteFile(full, []byte("test commit message\n"), 0o644); err != nil {
			t.Fatalf("write msg: %v", err)
		}
		return rel
	}

	// runGateE invokes commit-gate.sh with the given subcommand + args (cwd =
	// scratch repo). Extra env vars are appended (for the GC-cap test). Returns
	// parsed JSON (last {status:...} line), combined output, and any exec error.
	// Non-fatal so a goroutine can collect the error instead of fatal-calling.
	runGateE := func(dstScripts, dir string, env []string, args ...string) (map[string]any, string, error) {
		cmd := exec.Command("bash", append([]string{filepath.Join(dstScripts, "commit-gate.sh")}, args...)...)
		cmd.Dir = dir
		if len(env) > 0 {
			cmd.Env = append(os.Environ(), env...)
		}
		out, err := cmd.CombinedOutput()
		combined := string(out)
		var parsed map[string]any
		for _, line := range strings.Split(combined, "\n") {
			tl := strings.TrimSpace(line)
			if !strings.HasPrefix(tl, "{") || !strings.HasSuffix(tl, "}") {
				continue
			}
			var cand map[string]any
			if json.Unmarshal([]byte(tl), &cand) == nil {
				if _, ok := cand["status"]; ok {
					parsed = cand
				}
			}
		}
		return parsed, combined, err
	}
	runGate := func(t *testing.T, dstScripts, dir string, env []string, args ...string) map[string]any {
		t.Helper()
		parsed, combined, err := runGateE(dstScripts, dir, env, args...)
		if err != nil {
			t.Fatalf("commit-gate %v failed: %v\n%s", args, err, combined)
		}
		if parsed == nil {
			t.Fatalf("commit-gate %v produced no status JSON\n%s", args, combined)
		}
		return parsed
	}

	gitIn := func(t *testing.T, dir string, args ...string) string {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		out, err := cmd.Output()
		if err != nil {
			ec, _ := cmd.CombinedOutput()
			t.Fatalf("git %v: %v\n%s", args, err, ec)
		}
		return strings.TrimSpace(string(out))
	}

	seedAndCommit := func(t *testing.T, dir, body string) {
		t.Helper()
		full := filepath.Join(dir, "file.txt")
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatalf("seed: %v", err)
		}
		gitIn(t, dir, "add", "file.txt")
		gitIn(t, dir, "commit", "-q", "-m", "seed")
	}

	// readLedger parses every non-empty line of closeouts.log as JSON.
	readLedger := func(t *testing.T, dir string) []map[string]any {
		t.Helper()
		data, err := os.ReadFile(filepath.Join(dir, ".git", "commit-gate", "closeouts.log"))
		if err != nil {
			t.Fatalf("read closeouts.log: %v", err)
		}
		var records []map[string]any
		for _, line := range strings.Split(string(data), "\n") {
			tl := strings.TrimSpace(line)
			if tl == "" {
				continue
			}
			var rec map[string]any
			if err := json.Unmarshal([]byte(tl), &rec); err != nil {
				t.Fatalf("parse ledger line %q: %v", tl, err)
			}
			records = append(records, rec)
		}
		return records
	}

	// -----------------------------------------------------------------
	// Subtest 1: acquire→commit records post_commit_head (THE outcome guard).
	// -----------------------------------------------------------------
	t.Run("records_post_commit_head", func(t *testing.T) {
		dir, dstScripts := setupScratchRepo(t)
		seedAndCommit(t, dir, "v1\n")
		headAtAcquire := gitIn(t, dir, "rev-parse", "HEAD")
		if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte("v2\n"), 0o644); err != nil {
			t.Fatalf("modify: %v", err)
		}
		uuidA := genUUID(t, dstScripts)
		msgRel := writeAgentMsg(t, dir, uuidA)
		acq := runGate(t, dstScripts, dir, nil, "acquire",
			"--paths", `["file.txt"]`,
			"--message-file", msgRel,
			"--session-alias", "ledger-test")
		uuidG, _ := acq["uuid"].(string)
		treeHash, _ := acq["tree_hash"].(string)
		if status, _ := acq["status"].(string); status != "acquired" {
			t.Fatalf("expected status acquired, got %v", acq)
		}
		comm := runGate(t, dstScripts, dir, nil, "commit",
			"--uuid", uuidG,
			"--tree-hash", treeHash,
			"--message-file", msgRel)
		if status, _ := comm["status"].(string); status != "committed" {
			t.Fatalf("expected status committed, got %v", comm)
		}

		// The actual branch HEAD after the commit landed (ground truth).
		postHead := gitIn(t, dir, "rev-parse", "HEAD")

		records := readLedger(t, dir)
		if len(records) != 1 {
			t.Fatalf("expected exactly 1 ledger record after one commit, got %d", len(records))
		}
		rec := records[0]
		// Schema: every field present.
		for _, k := range []string{"uuid", "acquired_at", "head_at_acquire", "post_commit_head", "status", "branch", "ts"} {
			if _, ok := rec[k]; !ok {
				t.Errorf("ledger record missing field %q: %v", k, rec)
			}
		}
		// Outcome assertions (not mechanism).
		if got := rec["status"]; got != "committed" {
			t.Errorf("ledger status = %v, want committed", got)
		}
		if got := rec["post_commit_head"]; got != postHead {
			t.Errorf("ledger post_commit_head = %v, want actual HEAD %s", got, postHead)
		}
		if got := rec["head_at_acquire"]; got != headAtAcquire {
			t.Errorf("ledger head_at_acquire = %v, want acquire-time HEAD %s", got, headAtAcquire)
		}
		if got := rec["uuid"]; got != uuidG {
			t.Errorf("ledger uuid = %v, want gate session uuid %s", got, uuidG)
		}
		if got := rec["branch"]; got != "main" {
			t.Errorf("ledger branch = %v, want main", got)
		}
	})

	// -----------------------------------------------------------------
	// Subtest 2: the ledger survives the transient session-meta cleanup.
	// -----------------------------------------------------------------
	t.Run("ledger_survives_session_meta_cleanup", func(t *testing.T) {
		dir, dstScripts := setupScratchRepo(t)
		seedAndCommit(t, dir, "v1\n")
		if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte("v2\n"), 0o644); err != nil {
			t.Fatalf("modify: %v", err)
		}
		uuidA := genUUID(t, dstScripts)
		msgRel := writeAgentMsg(t, dir, uuidA)
		acq := runGate(t, dstScripts, dir, nil, "acquire",
			"--paths", `["file.txt"]`,
			"--message-file", msgRel,
			"--session-alias", "ledger-test")
		uuidG, _ := acq["uuid"].(string)
		treeHash, _ := acq["tree_hash"].(string)
		runGate(t, dstScripts, dir, nil, "commit",
			"--uuid", uuidG,
			"--tree-hash", treeHash,
			"--message-file", msgRel)

		// Transient session meta MUST be reclaimed (the existing behavior).
		metaPath := filepath.Join(dir, ".git", "commit-gate", "meta-"+uuidG)
		if _, err := os.Stat(metaPath); !os.IsNotExist(err) {
			t.Fatalf("transient meta-%s must be reclaimed after commit; stat err=%v", uuidG, err)
		}
		// BUT the durable ledger MUST remain (the durability property).
		ledgerPath := filepath.Join(dir, ".git", "commit-gate", "closeouts.log")
		if _, err := os.Stat(ledgerPath); err != nil {
			t.Fatalf("durable closeouts.log must SURVIVE session-meta cleanup; stat err=%v", err)
		}
		records := readLedger(t, dir)
		if len(records) != 1 {
			t.Fatalf("ledger must hold 1 record after the survived cleanup, got %d", len(records))
		}
	})

	// -----------------------------------------------------------------
	// Subtest 3: GC count-cap trims the append-only ledger to the tail.
	// -----------------------------------------------------------------
	t.Run("gc_count_cap_trims", func(t *testing.T) {
		dir, dstScripts := setupScratchRepo(t)
		seedAndCommit(t, dir, "v1\n")
		// Pre-seed the ledger with 250 sequential records (i > MAX=200).
		ledgerPath := filepath.Join(dir, ".git", "commit-gate", "closeouts.log")
		if err := os.MkdirAll(filepath.Dir(ledgerPath), 0o755); err != nil {
			t.Fatalf("mkdir gate dir: %v", err)
		}
		var buf strings.Builder
		for i := 1; i <= 250; i++ {
			// Each record carries a unique marker so we can prove tail-retention.
			line, _ := json.Marshal(map[string]any{
				"uuid": "seed", "acquired_at": "", "head_at_acquire": "",
				"post_commit_head": "", "status": "committed", "branch": "main",
				"ts": "seed-" + itoa(i),
			})
			buf.Write(line)
			buf.WriteByte('\n')
		}
		if err := os.WriteFile(ledgerPath, []byte(buf.String()), 0o644); err != nil {
			t.Fatalf("seed ledger: %v", err)
		}
		// Trigger _gate_gc_sweep via a no_changes acquire (file.txt unmodified
		// since seed → tree hash equals HEAD → no_changes → sweep runs).
		uuidA := genUUID(t, dstScripts)
		msgRel := writeAgentMsg(t, dir, uuidA)
		runGate(t, dstScripts, dir, []string{"COMMIT_GATE_CLOSEOUT_LOG_MAX=200"}, "acquire",
			"--paths", `["file.txt"]`,
			"--message-file", msgRel,
			"--session-alias", "gc-test")

		records := readLedger(t, dir)
		if len(records) != 200 {
			t.Fatalf("after sweep over cap, ledger must hold exactly 200 (tail) records, got %d", len(records))
		}
		// The tail keeps the MOST RECENT records: seeds 51..250 (drop=50).
		first := records[0]["ts"]
		if got, want := first, "seed-51"; got != want {
			t.Errorf("tail-trim first record ts = %v, want %v (the 51st seed; first 50 dropped)", got, want)
		}
		last := records[199]["ts"]
		if got, want := last, "seed-250"; got != want {
			t.Errorf("tail-trim last record ts = %v, want %v", got, want)
		}
	})

	// -----------------------------------------------------------------
	// Subtest 4: the initial-commit (zero-old-oid) success site records too.
	// (Closes the C-F1 gap: only the parented path was end-to-end covered.)
	// -----------------------------------------------------------------
	t.Run("records_initial_commit", func(t *testing.T) {
		dir, dstScripts := setupScratchRepo(t)
		// NO seed → unborn branch. Acquire the first content + commit (zero-old-oid path).
		if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte("first\n"), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
		uuidA := genUUID(t, dstScripts)
		msgRel := writeAgentMsg(t, dir, uuidA)
		acq := runGate(t, dstScripts, dir, nil, "acquire",
			"--paths", `["file.txt"]`,
			"--message-file", msgRel,
			"--session-alias", "init-test")
		uuidG, _ := acq["uuid"].(string)
		treeHash, _ := acq["tree_hash"].(string)
		if status, _ := acq["status"].(string); status != "acquired" {
			t.Fatalf("expected status acquired, got %v", acq)
		}
		comm := runGate(t, dstScripts, dir, nil, "commit",
			"--uuid", uuidG, "--tree-hash", treeHash, "--message-file", msgRel)
		if status, _ := comm["status"].(string); status != "committed" {
			t.Fatalf("expected status committed (initial), got %v", comm)
		}
		postHead := gitIn(t, dir, "rev-parse", "HEAD")
		records := readLedger(t, dir)
		if len(records) != 1 {
			t.Fatalf("expected 1 ledger record after initial commit, got %d", len(records))
		}
		if got := records[0]["post_commit_head"]; got != postHead {
			t.Errorf("initial-commit post_commit_head = %v, want HEAD %s", got, postHead)
		}
		if got := records[0]["status"]; got != "committed" {
			t.Errorf("initial-commit status = %v, want committed", got)
		}
	})

	// -----------------------------------------------------------------
	// Subtest 5: concurrent appends survive a concurrent count-cap trim
	// (B-F3 behavioral guard for the flock serialization). Three parallel
	// committers each land a DISTINCT file (no CAS merge conflict), against a
	// ledger pre-seeded at the cap, so every commit's sweep trims. The flock
	// must serialize append-vs-trim so no committed closeout is lost to a
	// concurrent trim's snapshot-to-mv window. This is a concurrency STRESS
	// guard (real parallelism + real CAS), not a deterministic scheduler proof;
	// the assertion is strong: the 3 newest records are the real commits and
	// must all survive any tail-trim (the tail keeps the most recent).
	// -----------------------------------------------------------------
	t.Run("concurrent_appends_survive_trim", func(t *testing.T) {
		if testing.Short() {
			t.Skip("concurrency stress test skipped in -short")
		}
		dir, dstScripts := setupScratchRepo(t)
		seedAndCommit(t, dir, "v1\n")
		// Pre-seed the ledger at the cap (200) so every concurrent commit's
		// sweep trims — exercising the append-vs-trim serialization path.
		ledgerPath := filepath.Join(dir, ".git", "commit-gate", "closeouts.log")
		if err := os.MkdirAll(filepath.Dir(ledgerPath), 0o755); err != nil {
			t.Fatalf("mkdir gate dir: %v", err)
		}
		var buf strings.Builder
		for i := 1; i <= 200; i++ {
			line, _ := json.Marshal(map[string]any{
				"uuid": "seed", "acquired_at": "", "head_at_acquire": "",
				"post_commit_head": "", "status": "committed", "branch": "main",
				"ts": "seed-" + itoa(i),
			})
			buf.Write(line)
			buf.WriteByte('\n')
		}
		if err := os.WriteFile(ledgerPath, []byte(buf.String()), 0o644); err != nil {
			t.Fatalf("seed ledger: %v", err)
		}

		const n = 3
		type job struct{ fileRel, msgRel string }
		jobs := make([]job, n)
		for i := 0; i < n; i++ {
			id := string(rune('a' + i))
			fileRel := "file-" + id + ".txt"
			if err := os.WriteFile(filepath.Join(dir, fileRel), []byte("content-"+id+"\n"), 0o644); err != nil {
				t.Fatalf("write %s: %v", fileRel, err)
			}
			uuidA := genUUID(t, dstScripts)
			msgRel := writeAgentMsg(t, dir, uuidA)
			jobs[i] = job{fileRel, msgRel}
		}

		// Acquire all 3 SEQUENTIALLY. acquire is serialized by the gate's
		// LOCK_DIR mutex, so parallel acquires only contend on that mutex (and
		// return contended/race_lost) — orthogonal to the ledger race under
		// test. acquire releases the lock dir immediately, so the 3 sessions
		// coexist cleanly. Store each session's gate uuid + tree hash.
		type sess struct{ uuidG, treeHash, msgRel string }
		sessions := make([]sess, n)
		for i := 0; i < n; i++ {
			j := jobs[i]
			paths := "[\"" + j.fileRel + "\"]"
			acq := runGate(t, dstScripts, dir, nil, "acquire",
				"--paths", paths, "--message-file", j.msgRel, "--session-alias", "conc-test")
			uuidG, _ := acq["uuid"].(string)
			treeHash, _ := acq["tree_hash"].(string)
			if status, _ := acq["status"].(string); status != "acquired" {
				t.Fatalf("acquire %s: %v", j.fileRel, acq)
			}
			sessions[i] = sess{uuidG, treeHash, j.msgRel}
		}

		// Commit all 3 in PARALLEL — concurrent update-ref CAS + closeout
		// append + count-cap sweep. THIS is where the flock must serialize
		// append-vs-trim so no committed closeout is lost to a concurrent
		// trim's snapshot-to-mv window (the B-F1/B-F2 race). The barrier
		// maximizes commit-phase overlap.
		gateUUIDs := make([]string, n)
		errs := make([]error, n)
		var wg sync.WaitGroup
		wg.Add(n)
		start := make(chan struct{})
		for i := 0; i < n; i++ {
			i := i
			go func() {
				defer wg.Done()
				<-start
				s := sessions[i]
				comm, combined, err := runGateE(dstScripts, dir, nil, "commit",
					"--uuid", s.uuidG, "--tree-hash", s.treeHash, "--message-file", s.msgRel)
				if err != nil || comm == nil {
					errs[i] = fmt.Errorf("commit: %v\n%s", err, combined)
					return
				}
				if status, _ := comm["status"].(string); status != "committed" {
					errs[i] = fmt.Errorf("commit status %v (want committed): %v", status, comm)
					return
				}
				gateUUIDs[i] = s.uuidG
			}()
		}
		close(start)
		wg.Wait()

		for i, err := range errs {
			if err != nil {
				t.Errorf("committer %d failed: %v", i, err)
			}
		}
		if t.Failed() {
			return
		}

		records := readLedger(t, dir)
		// The ledger tail-trims to the cap; the 3 real commits are the newest
		// records, so they MUST all be present (none lost to a trim race).
		present := make(map[string]bool, len(records))
		for _, r := range records {
			if u, _ := r["uuid"].(string); u != "" {
				present[u] = true
			}
		}
		for i, want := range gateUUIDs {
			if want == "" {
				t.Errorf("committer %d recorded no gate uuid", i)
				continue
			}
			if !present[want] {
				t.Errorf("committed closeout uuid %s (committer %d) MISSING from ledger — lost to a concurrent trim (flock serialization failed)", want, i)
			}
		}
		// Cap honored: the ledger never grew past the cap + concurrency slack.
		if len(records) > 200+n {
			t.Errorf("ledger count %d exceeds cap+%d slack; trim not running", len(records), n)
		}
	})

	// -----------------------------------------------------------------
	// Subtest 6: could_not_land on a genuine same-line merge conflict (the
	// Pattern-4 same-file tangle). This is the REAL gate-status crux for
	// sub-item 3 (disposition §4.2): a closeout that could not land because of
	// a content conflict, distinct from cas_conflict (concurrent HEAD movement
	// the CAS retry loop handles) and from error (gate-internal failure).
	// -----------------------------------------------------------------
	t.Run("could_not_land_on_merge_conflict", func(t *testing.T) {
		dir, dstScripts := setupScratchRepo(t)
		seedAndCommit(t, dir, "base\n") // single-line file; same-line edits conflict

		// Acquire A: working tree file.txt = "A\n", staged into A's private index.
		if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte("A\n"), 0o644); err != nil {
			t.Fatalf("write A: %v", err)
		}
		uuidAa := genUUID(t, dstScripts)
		msgA := writeAgentMsg(t, dir, uuidAa)
		acqA := runGate(t, dstScripts, dir, nil, "acquire",
			"--paths", `["file.txt"]`, "--message-file", msgA, "--session-alias", "conflict-a")
		uuidAg, _ := acqA["uuid"].(string)
		treeA, _ := acqA["tree_hash"].(string)
		if status, _ := acqA["status"].(string); status != "acquired" {
			t.Fatalf("A acquire: %v", acqA)
		}

		// Acquire B at the SAME HEAD: working tree file.txt = "B\n" (conflicting
		// same-line edit), staged into B's private index. Both sessions hold H0.
		if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte("B\n"), 0o644); err != nil {
			t.Fatalf("write B: %v", err)
		}
		uuidBa := genUUID(t, dstScripts)
		msgB := writeAgentMsg(t, dir, uuidBa)
		acqB := runGate(t, dstScripts, dir, nil, "acquire",
			"--paths", `["file.txt"]`, "--message-file", msgB, "--session-alias", "conflict-b")
		uuidBg, _ := acqB["uuid"].(string)
		treeB, _ := acqB["tree_hash"].(string)
		if status, _ := acqB["status"].(string); status != "acquired" {
			t.Fatalf("B acquire: %v", acqB)
		}

		// A commits first: lands on H0 (no movement yet) -> H1.
		commA := runGate(t, dstScripts, dir, nil, "commit",
			"--uuid", uuidAg, "--tree-hash", treeA, "--message-file", msgA)
		if status, _ := commA["status"].(string); status != "committed" {
			t.Fatalf("A commit should land first; got %v", commA)
		}

		// B commits: current_head=H1 (moved), CAS rebase 3-way merge CONFLICTS
		// (base="base", ours="B", theirs="A") -> could_not_land. Non-zero exit is
		// expected for a failure status; parse the JSON line regardless.
		commB, combinedB, errB := runGateE(dstScripts, dir, nil, "commit",
			"--uuid", uuidBg, "--tree-hash", treeB, "--message-file", msgB)
		if commB == nil {
			t.Fatalf("B commit produced no status JSON; err=%v\n%s", errB, combinedB)
		}
		if got := commB["status"]; got != "could_not_land" {
			t.Errorf("B commit status = %v, want could_not_land (same-line merge conflict)\n%s", got, combinedB)
		}
		// read-tree -m -i exits 0 on a content conflict (it leaves unmerged
		// stages); write-tree then fails on the unmerged index -> write_tree_failed.
		// Either merge_failed or write_tree_failed is an acceptable could_not_land
		// reason (both are the content-tangle class).
		if got := commB["reason"]; got != "write_tree_failed" && got != "merge_failed" {
			t.Errorf("B commit reason = %v, want write_tree_failed or merge_failed", got)
		}

		// The could_not_land closeout MUST be recorded into the durable ledger
		// so doctor's stall surfacing can see it.
		records := readLedger(t, dir)
		var foundB bool
		for _, r := range records {
			if r["uuid"] == uuidBg && r["status"] == "could_not_land" {
				foundB = true
				break
			}
		}
		if !foundB {
			t.Errorf("could_not_land closeout for B (uuid %s) not recorded in ledger; records: %v", uuidBg, records)
		}
	})
}

// itoa avoids fmt.Sprintf import noise in the seed loop above.
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var b [20]byte
	pos := len(b)
	for i > 0 {
		pos--
		b[pos] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		pos--
		b[pos] = '-'
	}
	return string(b[pos:])
}
