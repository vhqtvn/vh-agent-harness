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

// TestCommitGate_MessageCleanup is the committed bash-via-Go regression guard
// for commit-gate.sh message-scratch cleanup. It black-boxes the RENDERED
// .opencode/scripts/commit-gate.sh (NOT the template) inside a fully-isolated
// scratch git repo, mirroring TestCommitGate_BacklogSplitPreflight's harness:
// isolated .opencode/scripts/ copy, minimal valid opencode.jsonc + one
// agents/*.md, git init + initial commit, drive the real gate, assert on FS.
//
// The KEY regression guard is the two-UUID happy path (subtest two_uuid_happy_path):
// the gate's internal session UUID (returned by acquire) differs from the
// agent's pre-acquire gen-uuid used to NAME the msg scratch file. Before the
// 049d186 fix, the success path only removed ${MSG_SCRATCH_DIR}/msg-${GATE_UUID}
// and left the agent-authored ${MSG_SCRATCH_DIR}/msg-${AGENT_UUID} behind.
// This test asserts the agent file is reclaimed on commit / release / no_changes,
// and that the aged-GC backstop + protected-UUID skip behave correctly.
//
// Coverage (each subtest is a real regression vector):
//   - two_uuid_happy_path:        acquire→commit reclaims agent msg file (THE guard)
//   - release_reclaims_agent_file: acquire→release reclaims agent msg file
//     (exercises the --message-file flag added to release)
//   - no_changes_reclaims_agent_file: acquire with no diff reclaims agent msg file
//   - aged_orphan_gc_sweep:        aged msg-* orphans reaped by _gate_gc_sweep
//   - young_and_current_uuid_protected: fresh + _current_uuid files survive sweep
//   - release_rejects_arbitrary_message_file: B-F1 guard — release --message-file
//     with plain-arbitrary / parent-traversal / absolute-path vectors all rejected
//     by _safe_msg_reclaim_path chokepoint; victim file survives each
//   - commit_rejects_arbitrary_message_file: B-F1 latent vector via commit path
//     (commit forwards $message_file into _cleanup_own_scratch on success)
//   - release_rejects_traversal_uuid: B-UUID-F1 guard — release --uuid with
//     traversal payload rejected by ^[A-Za-z0-9_-]+$ uuid shape guard
func TestCommitGate_MessageCleanup(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not on PATH")
	}
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not on PATH")
	}

	// Rendered corpus scripts (repo-relative; go test cwd = internal/cli/).
	repoRoot := filepath.Join("..", "..")
	srcScripts := filepath.Join(repoRoot, ".opencode", "scripts")
	for _, f := range []string{"commit-gate.sh", "readonly-scripts.sh", "validate-opencode-config.py"} {
		if _, err := os.Stat(filepath.Join(srcScripts, f)); err != nil {
			t.Skipf("rendered script %s unavailable: %v (run `vh-agent-harness update` first)", f, err)
		}
	}

	// setupScratchRepo builds an isolated temp git repo with the rendered gate
	// scripts + minimal valid opencode config (so _config_validate passes) +
	// git init/identity. Returns the repo dir and the in-repo scripts dir.
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
		// Minimal valid opencode.jsonc (agent entry with non-empty description).
		if err := os.WriteFile(filepath.Join(dir, "opencode.jsonc"),
			[]byte(`{ "$schema": "https://opencode.ai/config.json", "agent": { "build": { "description": "test" } } }`),
			0o644); err != nil {
			t.Fatalf("write opencode.jsonc: %v", err)
		}
		// One valid agents/*.md (frontmatter with description + mode).
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
		gitIn("config", "user.email", "t@t")
		gitIn("config", "user.name", "t")
		gitIn("config", "commit.gpgsign", "false")

		return dir, dstScripts
	}

	// genUUID shells out to the in-repo readonly-scripts.sh gen-uuid (the same
	// path the committer uses to name the agent msg scratch file).
	genUUID := func(t *testing.T, dstScripts string) string {
		t.Helper()
		out, err := exec.Command("bash", filepath.Join(dstScripts, "readonly-scripts.sh"), "gen-uuid").Output()
		if err != nil {
			t.Fatalf("gen-uuid: %v", err)
		}
		return strings.TrimSpace(string(out))
	}

	// writeAgentMsg writes the agent-authored message scratch file at
	// tmp/commit-gate-message/msg-${uuid} inside the repo (mirroring the
	// committer's Write-tool path) and returns its repo-relative path.
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

	// runGate invokes commit-gate.sh with the given subcommand + args, cwd =
	// the scratch repo. Returns parsed JSON (last valid {status:...} line on
	// stdout) and the combined output for diagnostics. Fails on non-zero exit.
	runGate := func(t *testing.T, dstScripts, dir string, args ...string) map[string]any {
		t.Helper()
		cmd := exec.Command("bash", append([]string{filepath.Join(dstScripts, "commit-gate.sh")}, args...)...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		combined := string(out)
		// Find the last valid JSON object carrying a "status" key (the json_out
		// line is printed last; config-validation output, if any, precedes it).
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
		if err != nil {
			t.Fatalf("commit-gate %v failed: %v\n%s", args, err, combined)
		}
		if parsed == nil {
			t.Fatalf("commit-gate %v produced no status JSON\n%s", args, combined)
		}
		return parsed
	}

	// gitIn is a small per-subtest git helper (git -C dir ...).
	gitIn := func(t *testing.T, dir string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	// seedAndCommit writes file.txt with body, git-adds, and commits it at HEAD.
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
	// Subtest 1: two-UUID happy path (THE key regression guard).
	// ---------------------------------------------------------------------
	t.Run("two_uuid_happy_path", func(t *testing.T) {
		dir, dstScripts := setupScratchRepo(t)
		seedAndCommit(t, dir, "v1\n")
		// Modify after commit so there is a working-tree change to acquire.
		if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte("v2\n"), 0o644); err != nil {
			t.Fatalf("modify: %v", err)
		}

		// Agent authors a message scratch file with its OWN uuid (UUID_A).
		uuidA := genUUID(t, dstScripts)
		msgRel := writeAgentMsg(t, dir, uuidA)

		// acquire returns the GATE uuid (UUID_G), which MUST differ from UUID_A.
		acq := runGate(t, dstScripts, dir, "acquire",
			"--paths", `["file.txt"]`,
			"--message-file", msgRel,
			"--session-alias", "cleanup-test")
		uuidG, _ := acq["uuid"].(string)
		treeHash, _ := acq["tree_hash"].(string)
		if uuidG == "" {
			t.Fatalf("acquire did not return a uuid: %v", acq)
		}
		if uuidG == uuidA {
			t.Fatalf("test premise broken: gate uuid == agent uuid (%s); two-UUID guard would not exercise the regression", uuidG)
		}
		if status, _ := acq["status"].(string); status != "acquired" {
			t.Fatalf("expected status acquired, got %v", acq)
		}

		// commit with the gate uuid + the SAME agent message file.
		comm := runGate(t, dstScripts, dir, "commit",
			"--uuid", uuidG,
			"--tree-hash", treeHash,
			"--message-file", msgRel)
		if status, _ := comm["status"].(string); status != "committed" {
			t.Fatalf("expected status committed, got %v", comm)
		}

		// THE key assertion: agent-authored msg file is GONE after commit.
		// (This is exactly what would have failed before the 049d186 fix.)
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(msgRel))); !os.IsNotExist(err) {
			t.Fatalf("agent msg file %s must be reclaimed after commit; stat err=%v", msgRel, err)
		}
		// Gate-internal scratch for UUID_G must also be gone.
		for _, suffix := range []string{"msg-", "paths-"} {
			p := filepath.Join(dir, ".git", "commit-gate", suffix+uuidG)
			if _, err := os.Stat(p); !os.IsNotExist(err) {
				t.Fatalf(".git/commit-gate/%s%s must be reclaimed after commit; stat err=%v", suffix, uuidG, err)
			}
		}
	})

	// ---------------------------------------------------------------------
	// Subtest 2: release reclaims the agent msg file (exercises the new
	// --message-file flag added to release for this slice).
	// ---------------------------------------------------------------------
	t.Run("release_reclaims_agent_file", func(t *testing.T) {
		dir, dstScripts := setupScratchRepo(t)
		seedAndCommit(t, dir, "v1\n")
		if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte("v2\n"), 0o644); err != nil {
			t.Fatalf("modify: %v", err)
		}

		uuidA := genUUID(t, dstScripts)
		msgRel := writeAgentMsg(t, dir, uuidA)

		acq := runGate(t, dstScripts, dir, "acquire",
			"--paths", `["file.txt"]`,
			"--message-file", msgRel,
			"--session-alias", "cleanup-test")
		uuidG, _ := acq["uuid"].(string)
		if uuidG == "" {
			t.Fatalf("acquire did not return a uuid: %v", acq)
		}

		// release with the gate uuid + the agent message file (new flag).
		rel := runGate(t, dstScripts, dir, "release",
			"--uuid", uuidG,
			"--message-file", msgRel)
		if status, _ := rel["status"].(string); !strings.HasPrefix(status, "released") {
			t.Fatalf("expected status released*, got %v", rel)
		}

		// Agent msg file must be gone.
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(msgRel))); !os.IsNotExist(err) {
			t.Fatalf("agent msg file %s must be reclaimed after release; stat err=%v", msgRel, err)
		}
	})

	// ---------------------------------------------------------------------
	// Subtest 3: no_changes reclaims the agent msg file.
	// ---------------------------------------------------------------------
	t.Run("no_changes_reclaims_agent_file", func(t *testing.T) {
		dir, dstScripts := setupScratchRepo(t)
		// Seed + leave UNMODIFIED → tree hash equals HEAD → no diff.
		seedAndCommit(t, dir, "v1\n")

		uuidA := genUUID(t, dstScripts)
		msgRel := writeAgentMsg(t, dir, uuidA)

		acq := runGate(t, dstScripts, dir, "acquire",
			"--paths", `["file.txt"]`,
			"--message-file", msgRel,
			"--session-alias", "cleanup-test")
		if status, _ := acq["status"].(string); status != "no_changes" {
			t.Fatalf("expected status no_changes, got %v", acq)
		}

		// Agent msg file must be reclaimed by the no_changes cleanup branch.
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(msgRel))); !os.IsNotExist(err) {
			t.Fatalf("agent msg file %s must be reclaimed on no_changes; stat err=%v", msgRel, err)
		}
	})

	// ---------------------------------------------------------------------
	// Subtest 4: aged-orphan GC sweep reaps aged msg-* files in BOTH
	// scratch surfaces (agent-owned tmp/commit-gate-message/ + gate-internal
	// .git/commit-gate/). A no_changes acquire is the sweep trigger.
	// ---------------------------------------------------------------------
	t.Run("aged_orphan_gc_sweep", func(t *testing.T) {
		dir, dstScripts := setupScratchRepo(t)
		seedAndCommit(t, dir, "v1\n") // unmodified → no_changes acquire triggers sweep

		// Create aged orphans in BOTH scratch surfaces.
		orphanA := genUUID(t, dstScripts) // agent-owned surface
		agentOrphan := filepath.Join(dir, "tmp", "commit-gate-message", "msg-"+orphanA)
		if err := os.MkdirAll(filepath.Dir(agentOrphan), 0o755); err != nil {
			t.Fatalf("mkdir agent orphan dir: %v", err)
		}
		if err := os.WriteFile(agentOrphan, []byte("orphan\n"), 0o644); err != nil {
			t.Fatalf("write agent orphan: %v", err)
		}

		orphanB := genUUID(t, dstScripts) // gate-internal surface
		gateDir := filepath.Join(dir, ".git", "commit-gate")
		if err := os.MkdirAll(gateDir, 0o755); err != nil {
			t.Fatalf("mkdir gate dir: %v", err)
		}
		gateOrphan := filepath.Join(gateDir, "msg-"+orphanB)
		if err := os.WriteFile(gateOrphan, []byte("orphan\n"), 0o644); err != nil {
			t.Fatalf("write gate orphan: %v", err)
		}

		// Age both orphans past the GC threshold (DEFAULT_GC_MAX_AGE=3600).
		old := time.Now().Add(-2 * time.Hour)
		if err := os.Chtimes(agentOrphan, old, old); err != nil {
			t.Fatalf("chtimes agent orphan: %v", err)
		}
		if err := os.Chtimes(gateOrphan, old, old); err != nil {
			t.Fatalf("chtimes gate orphan: %v", err)
		}

		uuidA := genUUID(t, dstScripts)
		msgRel := writeAgentMsg(t, dir, uuidA)
		acq := runGate(t, dstScripts, dir, "acquire",
			"--paths", `["file.txt"]`,
			"--message-file", msgRel,
			"--session-alias", "cleanup-test")
		if status, _ := acq["status"].(string); status != "no_changes" {
			t.Fatalf("expected no_changes sweep trigger, got %v", acq)
		}

		// Both aged orphans must be reaped.
		if _, err := os.Stat(agentOrphan); !os.IsNotExist(err) {
			t.Fatalf("aged agent orphan must be reaped by GC; stat err=%v", err)
		}
		if _, err := os.Stat(gateOrphan); !os.IsNotExist(err) {
			t.Fatalf("aged gate orphan must be reaped by GC; stat err=%v", err)
		}
	})

	// ---------------------------------------------------------------------
	// Subtest 5: protected-UUID / fresh-file safety (negative guard).
	// A fresh (young) msg file must survive by the age gate; an AGED msg file
	// whose UUID is in _current_uuid must survive by the protected-UUID skip.
	// This guards against the GC becoming too aggressive.
	// ---------------------------------------------------------------------
	t.Run("young_and_current_uuid_protected", func(t *testing.T) {
		dir, dstScripts := setupScratchRepo(t)
		seedAndCommit(t, dir, "v1\n") // unmodified → no_changes acquire triggers sweep

		// youngUuid: fresh file (age gate protects it).
		youngUuid := genUUID(t, dstScripts)
		youngFile := filepath.Join(dir, "tmp", "commit-gate-message", "msg-"+youngUuid)
		if err := os.MkdirAll(filepath.Dir(youngFile), 0o755); err != nil {
			t.Fatalf("mkdir young dir: %v", err)
		}
		if err := os.WriteFile(youngFile, []byte("young\n"), 0o644); err != nil {
			t.Fatalf("write young: %v", err)
		}

		// currentUuid: AGED file but protected by _current_uuid (protected-UUID
		// skip must save it despite the age).
		currentUuid := genUUID(t, dstScripts)
		currentFile := filepath.Join(dir, "tmp", "commit-gate-message", "msg-"+currentUuid)
		if err := os.WriteFile(currentFile, []byte("current\n"), 0o644); err != nil {
			t.Fatalf("write current: %v", err)
		}
		old := time.Now().Add(-2 * time.Hour)
		if err := os.Chtimes(currentFile, old, old); err != nil {
			t.Fatalf("chtimes current: %v", err)
		}
		// Seed _current_uuid with currentUuid BEFORE acquire. The no_changes
		// path does NOT overwrite _current_uuid (it is only written on the
		// success path), so our seed survives through the sweep.
		gateDir := filepath.Join(dir, ".git", "commit-gate")
		if err := os.MkdirAll(gateDir, 0o755); err != nil {
			t.Fatalf("mkdir gate dir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(gateDir, "_current_uuid"), []byte(currentUuid), 0o644); err != nil {
			t.Fatalf("write _current_uuid: %v", err)
		}

		uuidA := genUUID(t, dstScripts)
		msgRel := writeAgentMsg(t, dir, uuidA)
		acq := runGate(t, dstScripts, dir, "acquire",
			"--paths", `["file.txt"]`,
			"--message-file", msgRel,
			"--session-alias", "cleanup-test")
		if status, _ := acq["status"].(string); status != "no_changes" {
			t.Fatalf("expected no_changes sweep trigger, got %v", acq)
		}

		// BOTH must survive: young by age gate, current by protected-UUID skip.
		if _, err := os.Stat(youngFile); err != nil {
			t.Fatalf("young msg file must SURVIVE the sweep (age gate); stat err=%v", err)
		}
		if _, err := os.Stat(currentFile); err != nil {
			t.Fatalf("current-uuid msg file must SURVIVE the sweep (protected-UUID skip); stat err=%v", err)
		}
	})

	// ---------------------------------------------------------------------
	// Subtest 6: release rejects an ARBITRARY --message-file (B-F1 guard).
	// release forwards $message_file into _cleanup_own_scratch's privileged
	// rm (rm is denied to all agents; the gate owns it). Before the
	// _safe_msg_reclaim_path chokepoint, `release --uuid <x> --message-file
	// <victim>` would delete any repo-relative file via the no-lock branch
	// (which needs no valid gate session). This subtest asserts the
	// chokepoint blocks plain-arbitrary, parent-traversal, and absolute-path
	// injection vectors — the victim file MUST survive each.
	// ---------------------------------------------------------------------
	releaseMustNotDelete := func(t *testing.T, mfArg string) {
		t.Helper()
		dir, dstScripts := setupScratchRepo(t)
		seedAndCommit(t, dir, "v1\n")
		if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte("v2\n"), 0o644); err != nil {
			t.Fatalf("modify: %v", err)
		}
		// victim file at repo root — must NOT be deleted by the release.
		victimRel := "victim.txt"
		if err := os.WriteFile(filepath.Join(dir, victimRel), []byte("victim\n"), 0o644); err != nil {
			t.Fatalf("write victim: %v", err)
		}
		// Legit acquire (creates tmp/commit-gate-message/ + a real session).
		uuidA := genUUID(t, dstScripts)
		msgRel := writeAgentMsg(t, dir, uuidA)
		acq := runGate(t, dstScripts, dir, "acquire",
			"--paths", `["file.txt"]`,
			"--message-file", msgRel,
			"--session-alias", "cleanup-test")
		uuidG, _ := acq["uuid"].(string)
		if uuidG == "" {
			t.Fatalf("acquire did not return a uuid: %v", acq)
		}
		// release with the MALICIOUS --message-file pointing at the victim.
		rel := runGate(t, dstScripts, dir, "release",
			"--uuid", uuidG,
			"--message-file", mfArg)
		if status, _ := rel["status"].(string); !strings.HasPrefix(status, "released") {
			t.Fatalf("expected status released*, got %v", rel)
		}
		// THE B-F1 assertion: victim survives the privileged rm.
		if _, err := os.Stat(filepath.Join(dir, victimRel)); err != nil {
			t.Fatalf("B-F1 regression: release --message-file %q deleted victim %s (chokepoint failed); stat err=%v", mfArg, victimRel, err)
		}
	}
	t.Run("release_rejects_arbitrary_message_file", func(t *testing.T) {
		absDir := t.TempDir() // only need a real path prefix for the absolute-path vector
		releaseMustNotDelete(t, "victim.txt")
		releaseMustNotDelete(t, filepath.ToSlash(filepath.Join("tmp", "commit-gate-message", "..", "..", "victim.txt")))
		releaseMustNotDelete(t, filepath.Join(absDir, "victim.txt"))
	})

	// ---------------------------------------------------------------------
	// Subtest 7: commit rejects an ARBITRARY --message-file (latent B-F1
	// vector via the commit path). commit also forwards $message_file into
	// _cleanup_own_scratch on success (the 049d186 fix). The same chokepoint
	// must block an arbitrary path there too. commit READS the file as the
	// commit message (cat), so the victim must exist; the guard is that it
	// is NOT deleted after the commit succeeds.
	// ---------------------------------------------------------------------
	t.Run("commit_rejects_arbitrary_message_file", func(t *testing.T) {
		dir, dstScripts := setupScratchRepo(t)
		seedAndCommit(t, dir, "v1\n")
		if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte("v2\n"), 0o644); err != nil {
			t.Fatalf("modify: %v", err)
		}
		victimRel := "victim.txt"
		if err := os.WriteFile(filepath.Join(dir, victimRel), []byte("victim-as-message\n"), 0o644); err != nil {
			t.Fatalf("write victim: %v", err)
		}
		// Legit acquire with the agent message file.
		uuidA := genUUID(t, dstScripts)
		msgRel := writeAgentMsg(t, dir, uuidA)
		acq := runGate(t, dstScripts, dir, "acquire",
			"--paths", `["file.txt"]`,
			"--message-file", msgRel,
			"--session-alias", "cleanup-test")
		uuidG, _ := acq["uuid"].(string)
		treeHash, _ := acq["tree_hash"].(string)
		if uuidG == "" {
			t.Fatalf("acquire did not return a uuid: %v", acq)
		}
		// commit with the MALICIOUS --message-file (victim is read as the
		// commit message, then would be rm'd on success without the chokepoint).
		comm := runGate(t, dstScripts, dir, "commit",
			"--uuid", uuidG,
			"--tree-hash", treeHash,
			"--message-file", victimRel)
		if status, _ := comm["status"].(string); status != "committed" {
			t.Fatalf("expected status committed, got %v", comm)
		}
		// The latent-vector guard: victim survives the privileged rm.
		if _, err := os.Stat(filepath.Join(dir, victimRel)); err != nil {
			t.Fatalf("B-F1 regression (commit path): commit --message-file %q deleted victim %s (chokepoint failed); stat err=%v", victimRel, victimRel, err)
		}
	})

	// ---------------------------------------------------------------------
	// Subtest 8: release rejects a TRAVERSAL --uuid (B-UUID-F1 guard).
	// _cleanup_own_scratch also runs rm -f ".../msg-${uuid}" using the FIRST
	// arg ($uuid) RAW. release's no-lock branch (reachable with NO valid gate
	// session) forwards the caller --uuid verbatim. Without a shape guard, a
	// traversal payload like 'x/../../../.git/config' — after the agent
	// pre-creates tmp/commit-gate-message/msg-x/ as a dir (Write-tool reachable)
	// — makes the privileged rm resolve out of the scratch dir and delete
	// <repo>/.git/config. The charset guard (^[A-Za-z0-9_-]+$, same convention
	// as cmd_stage_message) blocks this. This subtest exercises BOTH rm sinks
	// (GATE_INDEX_DIR/msg-x/ + MSG_SCRATCH_DIR/msg-x/ must both exist as dirs
	// for the traversal to resolve) and asserts .git/config survives.
	// ---------------------------------------------------------------------
	t.Run("release_rejects_traversal_uuid", func(t *testing.T) {
		dir, dstScripts := setupScratchRepo(t)
		seedAndCommit(t, dir, "v1\n")
		if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte("v2\n"), 0o644); err != nil {
			t.Fatalf("modify: %v", err)
		}
		// Pre-create the traversal dirs the payload climbs through. An agent
		// can create tmp/commit-gate-message/msg-x/ via the Write tool; we also
		// create .git/commit-gate/msg-x/ to exercise the GATE_INDEX_DIR sink.
		for _, d := range []string{
			filepath.Join(dir, "tmp", "commit-gate-message", "msg-x"),
			filepath.Join(dir, ".git", "commit-gate", "msg-x"),
		} {
			if err := os.MkdirAll(d, 0o755); err != nil {
				t.Fatalf("mkdir traversal dir %s: %v", d, err)
			}
		}
		// Legit acquire (establishes a session + the scratch dir).
		uuidA := genUUID(t, dstScripts)
		msgRel := writeAgentMsg(t, dir, uuidA)
		acq := runGate(t, dstScripts, dir, "acquire",
			"--paths", `["file.txt"]`,
			"--message-file", msgRel,
			"--session-alias", "cleanup-test")
		uuidG, _ := acq["uuid"].(string)
		if uuidG == "" {
			t.Fatalf("acquire did not return a uuid: %v", acq)
		}
		// release with the TRAVERSAL uuid. Without the guard, the privileged rm
		// resolves tmp/commit-gate-message/msg-x/../../../.git/config (and the
		// .git/commit-gate/msg-x/... sibling) to <repo>/.git/config and deletes it.
		rel := runGate(t, dstScripts, dir, "release",
			"--uuid", "x/../../../.git/config",
			"--message-file", msgRel)
		if status, _ := rel["status"].(string); !strings.HasPrefix(status, "released") {
			t.Fatalf("expected status released*, got %v", rel)
		}
		// THE B-UUID-F1 assertion: .git/config survives the privileged rm.
		if _, err := os.Stat(filepath.Join(dir, ".git", "config")); err != nil {
			t.Fatalf("B-UUID-F1 regression: release --uuid traversal deleted .git/config (uuid shape guard failed); stat err=%v", err)
		}
	})
}
