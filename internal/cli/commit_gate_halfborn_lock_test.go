package cli

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestCommitGate_HalfBornLockStaleBreak is the SAFETY-CRITICAL regression guard
// for the commit-gate-half-born-lock-stale-break DEFER card. It black-boxes the
// RENDERED .opencode/scripts/commit-gate.sh inside a fully-isolated scratch git
// repo, mirroring TestCommitGate_MessageCleanup / TestCommitGate_CloseoutLedger.
//
// THE BUG (half-born-lock-stale-break): during acquire's mkdir->meta birth
// window, $LOCK_DIR exists but $LOCK_META does not yet. _is_stale read the
// absent meta as "{}" (hb/pid/uuid all empty), skipped both guarded branches,
// fell through to the unconditional STALE_UUID=""; return 0 — declaring a
// half-born LIVE lock STALE. _stale_break was then called with expected_uuid=""
// — its verify-after-move compared actual_uuid ("") != expected_uuid ("") →
// FALSE → did NOT restore → rm -rf destroyed the LIVE lock → a second acquirer
// mkdir'd a fresh $LOCK_DIR and "acquired" → TWO HOLDERS, mutual exclusion
// broken. The class (per the card): absent treated as a legitimate value rather
// than unknown, collapsing "" == "" into a confirmed-match (the second
// confirmed instance after exec-sandbox ParseMinMode, fixed v0.21.0 63c5500).
//
// THE FIX (two guards, modeled on internal/cli/profile.go corpusDefaultFeatures
// — fail-safe, not fail-open):
//   - _is_stale: if the lock's uuid is absent/unparseable, the lock is being
//     born (or is corrupt); we cannot identify it for verify-after-move, so we
//     must NOT declare it stale. return 1.
//   - _stale_break: refuse outright when expected_uuid is empty (defense-in-
//     depth — _is_stale now blocks this path in the normal acquire flow, but a
//     future caller reaching here with an empty uuid must never move/destroy a
//     lock it cannot identify).
//
// Coverage (each subtest is a real behavioral outcome, not mechanism-assertion):
//   - halfborn_lock_not_broken:      THE crux — a half-born lockdir (mkdir done,
//     no meta) must NOT be destroyed by a concurrent acquire; acquire is refused
//     (contended) and the live lockdir SURVIVES. Proves mutual exclusion holds.
//   - is_stale_direct_halfborn:      _is_stale returns non-stale for a half-born
//     lock (asserts STALE_UUID stays empty). Direct unit test via sourced lib.
//   - stale_break_direct_empty_uuid: _stale_break refuses (rc!=0) and leaves the
//     lockdir in place when called with an empty expected_uuid. Direct unit test.
//   - genuinely_stale_still_breakable: recovery UNCHANGED — a lock with an
//     expired heartbeat on a different host (real staleness) IS broken and a
//     fresh acquire SUCCEEDS. Guards against weakening real recovery.
func TestCommitGate_HalfBornLockStaleBreak(t *testing.T) {
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

	// setupScratchRepo mirrors the harness in TestCommitGate_CloseoutLedger /
	// TestCommitGate_MessageCleanup: isolated temp git repo + rendered gate
	// scripts + minimal valid opencode config + git init/identity.
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
		gitIn("symbolic-ref", "HEAD", "refs/heads/main")
		gitIn("config", "user.email", "t@t")
		gitIn("config", "user.name", "t")
		gitIn("config", "commit.gpgsign", "false")
		return dir, dstScripts
	}

	// runGateE invokes commit-gate.sh with the given subcommand + args (cwd =
	// scratch repo). Returns parsed JSON (last {status:...} line), combined
	// output, and any exec error. Non-fatal so the caller can inspect a
	// contended/refused status that exits non-zero.
	runGateE := func(dstScripts, dir string, args ...string) (map[string]any, string, error) {
		cmd := exec.Command("bash", append([]string{filepath.Join(dstScripts, "commit-gate.sh")}, args...)...)
		cmd.Dir = dir
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

	// gitIn is a per-subtest git helper returning trimmed stdout.
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

	// ---------------------------------------------------------------------
	// Subtest 1 (THE crux): a half-born lock (LOCK_DIR present, NO meta) must
	// NOT be destroyed by a concurrent acquire. This is the mutual-exclusion
	// invariant: $LOCK_DIR existing IS owner A holding (mkdir is the
	// atomic-acquire). If B's acquire returned "acquired", A's live lock was
	// destroyed and B became a second holder — the safety break.
	// ---------------------------------------------------------------------
	t.Run("halfborn_lock_not_broken", func(t *testing.T) {
		dir, dstScripts := setupScratchRepo(t)
		seedAndCommit(t, dir, "v1\n")
		// Working-tree change so B has something to stage past validation.
		if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte("v2\n"), 0o644); err != nil {
			t.Fatalf("modify: %v", err)
		}
		// Agent message scratch (the blessed committer path).
		msgRel := filepath.ToSlash(filepath.Join("tmp", "commit-gate-message", "msg-halfborn"))
		if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, filepath.FromSlash(msgRel))), 0o755); err != nil {
			t.Fatalf("mkdir scratch: %v", err)
		}
		if err := os.WriteFile(filepath.Join(dir, filepath.FromSlash(msgRel)), []byte("halfborn\n"), 0o644); err != nil {
			t.Fatalf("write msg: %v", err)
		}

		// --- Simulate owner A mid-birth: mkdir done, meta NOT yet written. ---
		lockDir := filepath.Join(dir, ".git", "commit-gate.lock")
		if err := os.MkdirAll(lockDir, 0o755); err != nil {
			t.Fatalf("mkdir half-born lockdir: %v", err)
		}
		// Confirm the half-born state: dir present, meta absent.
		if _, err := os.Stat(lockDir); err != nil {
			t.Fatalf("half-born lockdir must exist: %v", err)
		}
		if _, err := os.Stat(filepath.Join(lockDir, "meta")); !os.IsNotExist(err) {
			t.Fatalf("test premise: half-born lock must have NO meta yet; stat err=%v", err)
		}

		// --- B runs acquire during A's birth window. ---
		// errB is intentionally ignored here: a contended/refused acquire
		// exits non-zero, and the assertion is on the parsed status, not rc.
		acqB, combinedB, _ := runGateE(dstScripts, dir, "acquire",
			"--paths", `["file.txt"]`,
			"--message-file", msgRel,
			"--session-alias", "halfborn-B")

		statusB, _ := acqB["status"].(string)

		// THE mutual-exclusion assertion: B must NOT have acquired. The only
		// acceptable outcome is "contended" (B refused because A's lock is
		// present and was correctly NOT declared stale). "acquired" is the
		// red signal — it means A's live lock was destroyed.
		if statusB == "acquired" {
			t.Fatalf("RED SIGNAL: mutual exclusion BROKEN — B acquired (status=acquired) while A's half-born lockdir was present.\n_is_stale declared the half-born lock STALE and _stale_break destroyed it (vacuous empty==empty verify).\noutput:\n%s", combinedB)
		}
		if statusB != "contended" {
			t.Fatalf("expected status contended (B refused), got %v\noutput:\n%s", acqB, combinedB)
		}

		// A's half-born lockdir MUST SURVIVE (not destroyed by stale-break).
		if _, err := os.Stat(lockDir); err != nil {
			t.Fatalf("REGRESSION: A's half-born lockdir was DESTROYED by B's acquire (mutual-exclusion break); stat err=%v", err)
		}
		// And no meta should have been written into it by B (B did not acquire).
		if _, err := os.Stat(filepath.Join(lockDir, "meta")); !os.IsNotExist(err) {
			t.Fatalf("REGRESSION: B wrote meta into A's half-born lockdir (B believes it holds A's lock); stat err=%v", err)
		}
		// A contended acquire exits non-zero; that is expected and fine — the
		// assertion is on the parsed status, not the exit code.
		t.Logf("B acquire correctly refused: status=%s (A's live lock preserved, mutual exclusion holds)", statusB)
	})

	// ---------------------------------------------------------------------
	// Subtest 2: _is_stale returns non-stale for a half-born lock, and leaves
	// STALE_UUID empty. Direct unit test by sourcing the gate's function
	// library (dispatcher stripped). Confirms the guard at the decision site.
	// ---------------------------------------------------------------------
	t.Run("is_stale_direct_halfborn", func(t *testing.T) {
		dir, dstScripts := setupScratchRepo(t)
		seedAndCommit(t, dir, "v1\n")

		// Build the function library: the rendered gate with the trailing
		// `main "$@"` dispatcher stripped, so sourcing defines the helpers
		// without running the CLI. The dispatcher is the literal last line.
		gateBytes, err := os.ReadFile(filepath.Join(dstScripts, "commit-gate.sh"))
		if err != nil {
			t.Fatalf("read gate: %v", err)
		}
		gateSrc := string(gateBytes)
		const dispatch = `main "$@"`
		if !strings.HasSuffix(strings.TrimRight(gateSrc, "\n"), dispatch) {
			t.Fatalf("gate does not end with the expected `%s` dispatcher; source layout changed", dispatch)
		}
		// Strip the trailing dispatcher line (and its newline).
		gateSrc = strings.TrimSuffix(gateSrc, dispatch+"\n")
		libPath := filepath.Join(dstScripts, "commit-gate-lib.sh")
		if err := os.WriteFile(libPath, []byte(gateSrc), 0o755); err != nil {
			t.Fatalf("write lib: %v", err)
		}

		// Half-born lock: dir present, NO meta.
		lockDirRel := ".git/commit-gate.lock"
		if err := os.MkdirAll(filepath.Join(dir, lockDirRel), 0o755); err != nil {
			t.Fatalf("mkdir half-born lockdir: %v", err)
		}

		// Source the lib (cwd = scratch repo so LOCK_DIR/LOCK_META resolve),
		// then call _is_stale directly. Under the lib's `set -euo pipefail`,
		// capture the return via an `if` so a non-stale (return 1) does not
		// kill the shell; echo the verdict + STALE_UUID on a single line.
		harness := `set -uo pipefail
source ./.opencode/scripts/commit-gate-lib.sh
set +e
if _is_stale "` + lockDirRel + `"; then
  echo "VERDICT stale STALE_UUID=$STALE_UUID"
else
  echo "VERDICT not_stale STALE_UUID=$STALE_UUID"
fi
`
		harnessPath := filepath.Join(dir, "is_stale_harness.sh")
		if err := os.WriteFile(harnessPath, []byte(harness), 0o755); err != nil {
			t.Fatalf("write harness: %v", err)
		}
		cmd := exec.Command("bash", "is_stale_harness.sh")
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("harness failed: %v\n%s", err, out)
		}
		got := strings.TrimSpace(string(out))
		t.Logf("_is_stale half-born direct: %s", got)

		// THE guard assertion: a half-born lock is NOT stale, and STALE_UUID
		// stays empty (no identity leaked to a would-be stale-break caller).
		if !strings.HasPrefix(got, "VERDICT not_stale") {
			t.Fatalf("REGRESSION: _is_stale declared a half-born lock STALE (the bug); got: %s", got)
		}
		// STALE_UUID must be empty (the reset at the top of _is_stale holds +
		// the new guard returns before the fall-through that would set it).
		if !strings.HasSuffix(got, "STALE_UUID=") {
			t.Fatalf("REGRESSION: _is_stale leaked a non-empty STALE_UUID for a half-born lock; got: %s", got)
		}
	})

	// ---------------------------------------------------------------------
	// Subtest 3 (defense-in-depth): _stale_break refuses (rc!=0) and leaves
	// the lockdir in place when called with an empty expected_uuid. Even
	// though _is_stale now blocks this path in the normal acquire flow, the
	// verify-after-move must NEVER accept the vacuous empty==empty match.
	// ---------------------------------------------------------------------
	t.Run("stale_break_direct_empty_uuid", func(t *testing.T) {
		dir, dstScripts := setupScratchRepo(t)
		seedAndCommit(t, dir, "v1\n")

		// Same function-library sourcing as subtest 2.
		gateBytes, err := os.ReadFile(filepath.Join(dstScripts, "commit-gate.sh"))
		if err != nil {
			t.Fatalf("read gate: %v", err)
		}
		gateSrc := string(gateBytes)
		const dispatch = `main "$@"`
		gateSrc = strings.TrimSuffix(gateSrc, dispatch+"\n")
		libPath := filepath.Join(dstScripts, "commit-gate-lib.sh")
		if err := os.WriteFile(libPath, []byte(gateSrc), 0o755); err != nil {
			t.Fatalf("write lib: %v", err)
		}

		// A HALF-BORN lockdir (mkdir done, NO meta) — the genuinely dangerous
		// case: with the bug, _stale_break moves it, reads actual_uuid="" ==
		// expected_uuid="" (vacuous match), does NOT restore, and rm -rf's it.
		// The guard must refuse BEFORE moving so the live lock is never touched.
		lockDirRel := ".git/commit-gate.lock"
		lockDir := filepath.Join(dir, lockDirRel)
		if err := os.MkdirAll(lockDir, 0o755); err != nil {
			t.Fatalf("mkdir half-born lockdir: %v", err)
		}
		// Confirm the half-born premise: dir present, meta absent.
		if _, err := os.Stat(filepath.Join(lockDir, "meta")); !os.IsNotExist(err) {
			t.Fatalf("test premise: half-born lock must have NO meta; stat err=%v", err)
		}

		harness := `set -uo pipefail
source ./.opencode/scripts/commit-gate-lib.sh
set +e
_stale_break "` + lockDirRel + `" ""
rc=$?
echo "RC=$rc"
echo "LOCKDIR_PRESENT=$([ -d "` + lockDirRel + `" ] && echo yes || echo no)"
`
		harnessPath := filepath.Join(dir, "stale_break_harness.sh")
		if err := os.WriteFile(harnessPath, []byte(harness), 0o755); err != nil {
			t.Fatalf("write harness: %v", err)
		}
		cmd := exec.Command("bash", "stale_break_harness.sh")
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("harness failed: %v\n%s", err, out)
		}
		got := string(out)
		t.Logf("_stale_break empty-uuid direct:\n%s", got)

		// THE defense-in-depth assertion: _stale_break must REFUSE (rc!=0) and
		// leave the half-born lockdir in place (no move, no destroy).
		if rcLine := findLine(got, "RC="); strings.TrimSpace(rcLine) == "RC=0" {
			t.Fatalf("REGRESSION: _stale_break accepted an empty expected_uuid (rc=0); it must refuse. got:\n%s", got)
		}
		if !strings.Contains(got, "LOCKDIR_PRESENT=yes") {
			t.Fatalf("REGRESSION: _stale_break moved/destroyed the half-born lockdir despite an empty expected_uuid; got:\n%s", got)
		}
	})

	// ---------------------------------------------------------------------
	// Subtest 4 (recovery unchanged): a GENUINELY stale lock (expired
	// heartbeat on a different host) MUST still be breakable, and a fresh
	// acquire MUST succeed. Guards against the fix weakening real recovery
	// (the card's ready_criteria #3 + non-goal #3).
	// ---------------------------------------------------------------------
	t.Run("genuinely_stale_still_breakable", func(t *testing.T) {
		dir, dstScripts := setupScratchRepo(t)
		seedAndCommit(t, dir, "v1\n")
		if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte("v2\n"), 0o644); err != nil {
			t.Fatalf("modify: %v", err)
		}
		msgRel := filepath.ToSlash(filepath.Join("tmp", "commit-gate-message", "msg-stale"))
		if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, filepath.FromSlash(msgRel))), 0o755); err != nil {
			t.Fatalf("mkdir scratch: %v", err)
		}
		if err := os.WriteFile(filepath.Join(dir, filepath.FromSlash(msgRel)), []byte("stale-recovery\n"), 0o644); err != nil {
			t.Fatalf("write msg: %v", err)
		}

		// A genuinely stale lock: expired heartbeat + different host (hits the
		// different-host stale branch deterministically regardless of pid
		// liveness). uuid is populated so verify-after-move has a real match.
		lockDir := filepath.Join(dir, ".git", "commit-gate.lock")
		if err := os.MkdirAll(lockDir, 0o755); err != nil {
			t.Fatalf("mkdir stale lockdir: %v", err)
		}
		// heartbeat_at = 2020 (well past TTL=600s). hostname = "stale-host"
		// (guaranteed != the test machine's real hostname).
		staleMeta := []byte(`{"version":2,"uuid":"stale-uuid-1234","acquired_at":"2020-01-01T00:00:00Z","heartbeat_at":"2020-01-01T00:00:00Z","pid":999999,"session_alias":"stale","hostname":"stale-host","tree_hash":null,"message":"stale","private_index":"","head_at_acquire":"","paths":[]}` + "\n")
		if err := os.WriteFile(filepath.Join(lockDir, "meta"), staleMeta, 0o644); err != nil {
			t.Fatalf("write stale meta: %v", err)
		}
		// Age the lockdir mtime past TTL too (defense; heartbeat is the
		// primary signal but keep the filesystem consistent).
		old := time.Now().Add(-2 * time.Hour)
		_ = os.Chtimes(lockDir, old, old)

		// B acquires: _is_stale must still declare this stale (uuid present,
		// heartbeat expired, different host), _stale_break destroys it (uuid
		// match), and B mkdir's a fresh lockdir → "acquired".
		acqB, combinedB, errB := runGateE(dstScripts, dir, "acquire",
			"--paths", `["file.txt"]`,
			"--message-file", msgRel,
			"--session-alias", "stale-recovery-B")
		if errB != nil {
			t.Fatalf("genuinely-stale acquire should succeed (stale break + fresh acquire); got err=%v\n%s", errB, combinedB)
		}
		statusB, _ := acqB["status"].(string)
		if statusB != "acquired" {
			t.Fatalf("REGRESSION: genuinely-stale lock was NOT broken (recovery weakened by the fix); got status=%v\n%s", acqB, combinedB)
		}
		// The stale lockdir was replaced by B's fresh one (which acquire then
		// releases immediately at the end of the acquire phase). Confirm B got
		// a real session uuid (proof B actually holds a fresh lock).
		uuidB, _ := acqB["uuid"].(string)
		if uuidB == "" {
			t.Fatalf("REGRESSION: stale-break acquire returned no uuid; got %v", acqB)
		}
		t.Logf("genuinely-stale recovery unchanged: stale lock broken, B acquired (uuid=%s)", uuidB)
	})
}

// findLine returns the first line of `out` that contains `marker`, trimmed.
func findLine(out, marker string) string {
	for _, l := range strings.Split(out, "\n") {
		tl := strings.TrimSpace(l)
		if strings.Contains(tl, marker) {
			return tl
		}
	}
	return ""
}
