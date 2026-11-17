package cli

// Historical-M wrapper tests for scripts/release-tag.sh.
//
// The historical-M flow covers the case where HEAD has advanced past a
// completed ceremony's manifest commit M (classically: an operator-directed
// chore commit lands after M). The wrapper DERIVES the eligible M from
// current committed ceremony anchors (H's manifest -> R := evaluated_commit
// -> M := R's direct single-parent child on H's first-parent chain) and, if
// eligible, verifies M in a wrapper-owned detached verification worktree X:
//
//   - G0 (go test/vet/build/gofmt) runs INSIDE X at M — a defect at M
//     refuses even when repaired at H;
//   - G0b is checked on the ORIGINAL checkout before isolation and again at
//     the mutation boundary (a dirty original refuses no matter how clean X
//     is; a clean X cannot substitute);
//   - G0c control run stays on the original checkout (full doctor at H);
//     the target run points the ceremony binary at X with --json and the
//     release-context classes (target|both FAIL refuses; control FAILs are
//     ignored there; unclassified fails coverage). Doctor's REAL exit
//     contract on that target run is nonzero-on-problems — the FAIL entries
//     the router is going to ignore still exit non-zero with the full JSON
//     envelope on stdout — so the wrapper captures the envelope regardless
//     of exit code and refuses only on spawn failure / unparseable output.
//     G0c applicability is PER-SIDE (round-2 activation fix): the control
//     run gates on lineage.yml at H, the target run gates on the
//     lineage.yml BLOB at M (git cat-file -e M:...), and an M..H edit or
//     deletion of lineage.yml is a protected-path refusal;
//   - G7/G1-G5/G6 evaluators anchor at M via --commit;
//   - --at <full-SHA> is an ASSERTION that must equal the derived eligible
//     M — never a free target selector (superseded ceremonies refuse);
//   - eligibility: no suffix merges, no protected-path changes in ANY
//     intervening commit (edit, deletion, or revert — lineage.yml included),
//     identical manifest/readiness/migration-note blobs at M and H;
//   - the tag peels to exactly M; the original HEAD/branch/index never move.
//
// The real-history arms pin the derivation against the recorded v0.29.0
// chains: at H=7f94070a the original M1 (a9eb029f) is identifiable; at
// H=M2 (dc6697c) --at <M1> must refuse (superseded).

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

// releaseTagHistoricalResult deserializes the wrapper JSON including the
// historical-M extension fields (head_drift, historical).
type releaseTagHistoricalResult struct {
	OK                bool                     `json:"ok"`
	Tag               *string                  `json:"tag"`
	Commit            *string                  `json:"commit"`
	Pushed            bool                     `json:"pushed"`
	Error             *string                  `json:"error"`
	Disclosures       []map[string]interface{} `json:"disclosures"`
	AcceptedOverrides []map[string]interface{} `json:"accepted_overrides"`
	HeadDrift         *bool                    `json:"head_drift"`
	Historical        *bool                    `json:"historical"`
}

// runReleaseTagHistorical runs the wrapper with an explicit cwd and extra
// environment (the shared runReleaseTagManifest pins cwd to <scratch> and
// offers no env hook, both of which the historical tests need).
func runReleaseTagHistorical(t *testing.T, wrapper, cwd, msgFile, version string, extraArgs []string, extraEnv map[string]string, timeout time.Duration) (int, releaseTagHistoricalResult, string, string) {
	t.Helper()
	args := []string{wrapper, version}
	args = append(args, extraArgs...)
	cmd := exec.Command("bash", args...)
	cmd.Dir = cwd
	env := os.Environ()
	env = append(env, "RELEASE_TAG_MESSAGE_FILE="+msgFile)
	for k, v := range extraEnv {
		env = append(env, k+"="+v)
	}
	cmd.Env = env
	var outb, errb strings.Builder
	cmd.Stdout = &outb
	cmd.Stderr = &errb
	timer := time.AfterFunc(timeout, func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
	})
	defer timer.Stop()
	runErr := cmd.Run()
	exitCode := 0
	if runErr != nil {
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			t.Fatalf("bash spawn error: %v\nstderr: %s", runErr, errb.String())
		}
	}
	stdout := outb.String()
	stderr := errb.String()
	var result releaseTagHistoricalResult
	if stdout != "" {
		if err := json.Unmarshal([]byte(stdout), &result); err != nil {
			t.Fatalf("wrapper output must be valid JSON (exit=%d): %v\nstdout:\n%s\nstderr:\n%s",
				exitCode, err, stdout, stderr)
		}
	}
	return exitCode, result, stdout, stderr
}

// historicalFixture is the scratch state a historical test needs: the
// completed ceremony (N -> R -> M) plus the post-M chore commit H.
type historicalFixture struct {
	scratch string
	wrapper string
	spec    manifestSpec
	nsha    string // note commit (M^^)
	rsha    string // artifact commit (M^)
	msha    string // manifest commit M (the derived eligible anchor)
	hsha    string // chore commit H (HEAD after the chore)
	branch  string // current branch name
}

// scratchHooksTestGo is the env-gated hook module planted into the scratch
// repo for the lifecycle tests. It runs as part of the wrapper's G0
// `go test ./...` INSIDE the verification worktree X (cwd = X's module
// root, so ../.. is the ORIGINAL scratch repo and ../../tmp is its tmp/).
// Both hooks fire only when the driving test sets the matching env var:
//
//   - SCRATCH_HIST_LOCK_TMP=1: chmod 0555 the original repo's tmp/, so the
//     pre-tag cleanup cannot unlink X from its parent — the cleanup-failure
//     refusal arm. Inert under -run filters that never reach the wrapper.
//   - SCRATCH_HIST_MOVE_HEAD=1: commit a concurrent file on the ORIGINAL
//     repo mid-window — the final historical drift refusal arm.
const scratchHooksTestGo = `package scratch

import (
	"os"
	"os/exec"
	"testing"
)

// TestScratchHistHook is env-gated fixture machinery planted by
// release_tag_historical_test.go; it is inert unless the driving test sets
// the matching environment variable.
func TestScratchHistHook(t *testing.T) {
	if os.Getenv("SCRATCH_HIST_LOCK_TMP") == "1" {
		if os.Geteuid() == 0 {
			t.Skip("root: chmod would not block cleanup")
		}
		// Lock the verification worktree ITSELF (cwd of this test binary is
		// X's module root), not the original repo's tmp/ — the S2 evaluator
		// legitimately mkdirs scratch under the original tmp/ and must keep
		// working; only the pre-tag cleanup of X must fail.
		if err := os.Chmod(".", 0o555); err != nil {
			t.Fatalf("lock X: %v", err)
		}
		return
	}
	if os.Getenv("SCRATCH_HIST_MOVE_HEAD") == "1" {
		if err := os.WriteFile("../../fileB2.go", []byte("package scratch\n\n// ConcurrentCommit landed during the tag window.\nconst ConcurrentCommit = true\n"), 0o644); err != nil {
			t.Fatalf("write fileB2: %v", err)
		}
		for _, args := range [][]string{
			{"-C", "../..", "add", "fileB2.go"},
			{"-C", "../..", "commit", "-q", "-m", "concurrent commit during tag window"},
		} {
			cmd := exec.Command("git", args...)
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("git %v: %v\n%s", args, err, out)
			}
		}
	}
}
`

// historicalFixtureOpts toggles the optional commits below the ceremony.
type historicalFixtureOpts struct {
	// Lineage commits .vh-agent-harness/lineage.yml so the wrapper's G0c
	// block activates (doctor runs; used with a fake or real ceremony
	// binary).
	Lineage bool

	// Hooks plants the env-gated hooks_test.go module (lifecycle arms).
	Hooks bool

	// BrokenGo, when non-empty, is committed as broken.go BEFORE the
	// ceremony (so the defect exists at M). Pair with a chore that fixes
	// it via FixBrokenGo to build the broken-M/green-H case.
	BrokenGo    string
	FixBrokenGo string

	// PreNoteRel/PreNoteContent, when set, commit an extra path before the
	// note commit (used to plant the protected-path revert source).
	PreNoteRel     string
	PreNoteContent string

	// ChoreRel/ChoreContent override the default post-M chore commit
	// (fileB.go const). Empty ChoreRel means the default chore.
	ChoreRel     string
	ChoreContent string

	// NoChore stops the fixture at M (H == M) for tests that build their
	// own suffix above M.
	NoChore bool
}

// setupHistoricalFixture builds a scratch repo with a COMPLETED ceremony
// (note N -> artifact R -> manifest M) and, by default, one eligible chore
// commit H above M. tmp/ is gitignored (so the verification worktree under
// tmp/ never dirties the final G0b) and scratch/tmp exists (the wrapper's
// canonical-parent check requires it).
func setupHistoricalFixture(t *testing.T, opts historicalFixtureOpts) *historicalFixture {
	t.Helper()
	spec := manifestSpecForReadiness()
	scratch, wrapper, _, _, _ := setupReleaseTagManifestRepo(t, spec)
	fx := &historicalFixture{scratch: scratch, wrapper: wrapper, spec: spec}

	// tmp/ must be ignored BEFORE any worktree lands under it (the final
	// G0b re-check on the original checkout would otherwise see `?? tmp/`).
	commitScratchFile(t, scratch, ".gitignore", "bin/\ntmp/\n", "gitignore tmp scratch")

	if opts.Lineage {
		linBody := "lineage_version: \"1\"\ntemplate:\n    source: templates/core\n    commit: \"\"\n    ref: test\nrender:\n    rendered_by: test\n"
		commitScratchFile(t, scratch, ".vh-agent-harness/lineage.yml", linBody, "lineage for historical test")
	}
	if opts.Hooks {
		commitScratchFile(t, scratch, "hooks_test.go", scratchHooksTestGo, "env-gated fixture hooks")
	}
	if opts.BrokenGo != "" {
		commitScratchFile(t, scratch, "broken.go", opts.BrokenGo, "plant defect below the ceremony")
	}
	if opts.PreNoteRel != "" {
		commitScratchFile(t, scratch, opts.PreNoteRel, opts.PreNoteContent, "pre-note fixture commit")
	}

	// Ceremony: note (N) -> readiness artifact (R) -> manifest (M).
	commitScratchFile(t, scratch, "templates/migrations/v0.2.0.md", "# v0.2.0\n\nTest migration note for the historical fixture.\n", "migration note (N)")
	fx.nsha = gitRevParseVerify(t, scratch, "HEAD")
	artifactBytes := buildReadinessArtifactBytes(t, readinessArtifactSpec{CommitSHA: fx.nsha})
	commitScratchFile(t, scratch, ".vh-agent-harness/release-readiness-pass.json", string(artifactBytes), "readiness artifact (R)")
	fx.rsha = gitRevParseVerify(t, scratch, "HEAD")
	restampManifest(t, scratch, spec)
	fx.msha = gitRevParseVerify(t, scratch, "HEAD")

	// The wrapper resolves the canonical scratch parent tmp/ — create it.
	if err := os.MkdirAll(filepath.Join(scratch, "tmp"), 0o755); err != nil {
		t.Fatalf("mkdir scratch tmp: %v", err)
	}

	branchOut, err := exec.Command("git", "-C", scratch, "symbolic-ref", "--short", "HEAD").Output()
	if err != nil {
		t.Fatalf("symbolic-ref: %v", err)
	}
	fx.branch = strings.TrimSpace(string(branchOut))

	if !opts.NoChore {
		fx.commitChore(t, opts.ChoreRel, opts.ChoreContent, opts.FixBrokenGo)
	}
	return fx
}

// commitChore adds the post-M commit. With all-empty overrides it commits
// the default eligible chore (a fileB.go const); fixBroken, when non-empty,
// instead rewrites broken.go to that (valid) content — the broken-at-M/
// green-at-H arm. The chore target may be a protected path (the
// protected-edit refusal arm) — callers pick it deliberately.
func (fx *historicalFixture) commitChore(t *testing.T, rel, content, fixBroken string) {
	t.Helper()
	if fixBroken != "" {
		commitScratchFile(t, fx.scratch, "broken.go", fixBroken, "chore: fix the defect after M")
	} else if rel != "" {
		commitScratchFile(t, fx.scratch, rel, content, "chore after M")
	} else {
		commitScratchFile(t, fx.scratch, "fileB.go", "package scratch\n\n// ChoreAfterM marks the eligible post-ceremony chore.\nconst ChoreAfterM = true\n", "chore after M")
	}
	fx.hsha = gitRevParseVerify(t, fx.scratch, "HEAD")
}

// histMsgFile stages a tag message file outside the scratch repo.
func histMsgFile(t *testing.T, version string) string {
	t.Helper()
	msgFile := filepath.Join(t.TempDir(), "msg.txt")
	if err := os.WriteFile(msgFile, []byte("release "+version+"\n\n-test\n"), 0o644); err != nil {
		t.Fatalf("write msg: %v", err)
	}
	return msgFile
}

// removeScratchFile deletes a tracked file within scratch and commits the
// deletion. commitScratchFile can only write content; the protected-path scan
// arms need real deletions (git diff --name-only lists deleted paths, so a
// deletion counts as a protected-path hit exactly like an edit).
func removeScratchFile(t *testing.T, scratch, rel, msg string) {
	t.Helper()
	cmd := exec.Command("git", "-C", scratch, "rm", "-q", "--", rel)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git rm %s: %v\n%s", rel, err, out)
	}
	cmd = exec.Command("git", "-C", scratch, "commit", "-q", "-m", msg)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit (rm %s): %v\n%s", rel, err, out)
	}
}

// worktreeEntryCount counts registered worktrees (porcelain "worktree "
// lines). A clean scratch has exactly 1 (the main checkout).
func worktreeEntryCount(t *testing.T, repo string) int {
	t.Helper()
	out, err := exec.Command("git", "-C", repo, "worktree", "list", "--porcelain").Output()
	if err != nil {
		t.Fatalf("worktree list: %v", err)
	}
	n := 0
	for _, ln := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(ln, "worktree ") {
			n++
		}
	}
	return n
}

// releaseVerifyLeftovers globs the wrapper-owned scratch prefix for
// worktree directories that should not survive a completed (or cleanly
// refused) run.
func releaseVerifyLeftovers(t *testing.T, fx *historicalFixture) []string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(fx.scratch, "tmp", "release-verify-*"))
	if err != nil {
		t.Fatalf("glob release-verify: %v", err)
	}
	return matches
}

// requireCleanHistoricalRepo asserts the original checkout is byte-for-byte
// where the run left it: same HEAD, same branch, clean status, and nothing
// above the captured H.
func requireCleanHistoricalRepo(t *testing.T, fx *historicalFixture) {
	t.Helper()
	if got := gitRevParseVerify(t, fx.scratch, "HEAD"); got != fx.hsha {
		t.Errorf("original HEAD moved: %s != %s", got, fx.hsha)
	}
	branchOut, err := exec.Command("git", "-C", fx.scratch, "symbolic-ref", "--short", "HEAD").Output()
	if err != nil {
		t.Fatalf("symbolic-ref: %v", err)
	}
	if got := strings.TrimSpace(string(branchOut)); got != fx.branch {
		t.Errorf("branch changed: %s != %s", got, fx.branch)
	}
	stOut, err := exec.Command("git", "-C", fx.scratch, "status", "--short").Output()
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if len(strings.TrimSpace(string(stOut))) != 0 {
		t.Errorf("worktree/index dirty after run:\n%s", stOut)
	}
	// No new ceremony commits above H.
	above, err := exec.Command("git", "-C", fx.scratch, "rev-list", "--count", fx.hsha+"..HEAD").Output()
	if err != nil {
		t.Fatalf("rev-list: %v", err)
	}
	if strings.TrimSpace(string(above)) != "0" {
		t.Errorf("commits landed above H during the run: %s", above)
	}
}

// =============================================================================
// ELIGIBLE HISTORICAL M — ACCEPTS
// =============================================================================

// TestReleaseTag_Historical_EligibleMAndChore_Allows — the core historical
// arm: an eligible ceremony at M with one benign chore commit above it
// tags agent-only. The tag peels to EXACTLY M, the original checkout is
// untouched, no new ceremony commits land, and the wrapper cleans up its
// verification worktree.
func TestReleaseTag_Historical_EligibleMAndChore_Allows(t *testing.T) {
	fx := setupHistoricalFixture(t, historicalFixtureOpts{})
	msgFile := histMsgFile(t, "v0.2.0")

	exitCode, result, _, stderr := runReleaseTagHistorical(t, fx.wrapper, fx.scratch, msgFile, "v0.2.0", nil, nil, 120*time.Second)
	if exitCode != 0 {
		t.Fatalf("eligible historical M must ALLOW (exit 0); got %d\nstderr:\n%s", exitCode, stderr)
	}
	if !result.OK {
		t.Fatalf("expected ok=true; error=%v", result.Error)
	}
	if result.Commit == nil || *result.Commit != fx.msha {
		t.Errorf("commit must be the derived M %s; got %v", fx.msha, result.Commit)
	}
	if result.Historical == nil || !*result.Historical {
		t.Errorf("historical must be true; got %v", result.Historical)
	}
	if result.HeadDrift == nil || *result.HeadDrift {
		t.Errorf("head_drift must be false; got %v", result.HeadDrift)
	}
	// Annotated tag peeling to exactly M.
	if !tagExists(t, fx.scratch, "v0.2.0") {
		t.Fatalf("tag v0.2.0 must exist")
	}
	peeled := gitRevParseVerify(t, fx.scratch, "refs/tags/v0.2.0^{commit}")
	if peeled != fx.msha {
		t.Errorf("tag must peel to M %s; got %s", fx.msha, peeled)
	}
	typOut, err := exec.Command("git", "-C", fx.scratch, "cat-file", "-t", "refs/tags/v0.2.0").Output()
	if err != nil {
		t.Fatalf("cat-file -t: %v", err)
	}
	if got := strings.TrimSpace(string(typOut)); got != "tag" {
		t.Errorf("tag must be annotated (cat-file -t=tag); got %s", got)
	}
	// Disclosures from M's manifest are still surfaced.
	if len(result.Disclosures) != 1 {
		t.Errorf("disclosures must carry the manifest record; got %v", result.Disclosures)
	}
	// Original checkout untouched; no wrapper leftovers.
	requireCleanHistoricalRepo(t, fx)
	if got := worktreeEntryCount(t, fx.scratch); got != 1 {
		t.Errorf("worktree list must be back to exactly 1 entry; got %d", got)
	}
	if leftovers := releaseVerifyLeftovers(t, fx); len(leftovers) != 0 {
		t.Errorf("verification worktree leaked: %v", leftovers)
	}
}

// TestReleaseTag_Historical_AtAssertionMatchesDerivedM_Allows — --at equal
// to the derived eligible M is a passing assertion, not a selector.
func TestReleaseTag_Historical_AtAssertionMatchesDerivedM_Allows(t *testing.T) {
	fx := setupHistoricalFixture(t, historicalFixtureOpts{})
	msgFile := histMsgFile(t, "v0.2.0")

	exitCode, result, _, _ := runReleaseTagHistorical(t, fx.wrapper, fx.scratch, msgFile, "v0.2.0",
		[]string{"--at", fx.msha}, nil, 120*time.Second)
	if exitCode != 0 || !result.OK {
		t.Fatalf("--at == derived M must ALLOW; exit=%d ok=%v error=%v", exitCode, result.OK, result.Error)
	}
	if result.Commit == nil || *result.Commit != fx.msha {
		t.Errorf("commit must be M %s; got %v", fx.msha, result.Commit)
	}
	if result.Historical == nil || !*result.Historical {
		t.Errorf("historical must be true; got %v", result.Historical)
	}
	requireCleanHistoricalRepo(t, fx)
}

// =============================================================================
// --at ASSERTION — wrong/malformed values refuse
// =============================================================================

// TestReleaseTag_Historical_AtAssertionWrongSHA_Refuses — a --at that does
// not equal the derived eligible M refuses with the derivation named.
func TestReleaseTag_Historical_AtAssertionWrongSHA_Refuses(t *testing.T) {
	fx := setupHistoricalFixture(t, historicalFixtureOpts{})
	msgFile := histMsgFile(t, "v0.2.0")

	exitCode, result, _, _ := runReleaseTagHistorical(t, fx.wrapper, fx.scratch, msgFile, "v0.2.0",
		[]string{"--at", fx.rsha}, nil, 60*time.Second)
	if exitCode == 0 {
		t.Fatalf("--at != derived M must REFUSE; got exit 0")
	}
	if result.OK {
		t.Errorf("expected ok=false")
	}
	if result.Error == nil ||
		!strings.Contains(*result.Error, "--at assertion failed") ||
		!strings.Contains(*result.Error, fx.msha) ||
		!strings.Contains(*result.Error, fx.rsha) {
		t.Errorf("error must name the assertion mismatch and both SHAs; got %s", *result.Error)
	}
	if tagExists(t, fx.scratch, "v0.2.0") {
		t.Errorf("tag must NOT exist after --at refusal")
	}
	if result.Historical == nil || !*result.Historical {
		t.Errorf("historical must be true on a post-derivation refusal; got %v", result.Historical)
	}
}

// TestReleaseTag_Historical_AtAssertionMalformed_Refuses — non-SHA --at
// values refuse before any derivation-dependent work.
func TestReleaseTag_Historical_AtAssertionMalformed_Refuses(t *testing.T) {
	fx := setupHistoricalFixture(t, historicalFixtureOpts{})
	msgFile := histMsgFile(t, "v0.2.0")

	exitCode, result, _, _ := runReleaseTagHistorical(t, fx.wrapper, fx.scratch, msgFile, "v0.2.0",
		[]string{"--at", "HEAD~1"}, nil, 60*time.Second)
	if exitCode == 0 {
		t.Fatalf("malformed --at must REFUSE; got exit 0")
	}
	if result.OK {
		t.Errorf("expected ok=false")
	}
	if result.Error == nil || !strings.Contains(*result.Error, "full 40-char lowercase hex") {
		t.Errorf("error must explain the --at SHA format; got %s", *result.Error)
	}
	if tagExists(t, fx.scratch, "v0.2.0") {
		t.Errorf("tag must NOT exist")
	}
}

// TestReleaseTag_Historical_PushOnlyAtCombination_Refuses — --at is a
// create-flow assertion; combining it with --push-only refuses.
func TestReleaseTag_Historical_PushOnlyAtCombination_Refuses(t *testing.T) {
	fx := setupHistoricalFixture(t, historicalFixtureOpts{})
	msgFile := histMsgFile(t, "v0.2.0")

	exitCode, result, _, _ := runReleaseTagHistorical(t, fx.wrapper, fx.scratch, msgFile, "v0.2.0",
		[]string{"--push-only", "--at", fx.msha}, nil, 60*time.Second)
	if exitCode == 0 {
		t.Fatalf("--push-only + --at must REFUSE; got exit 0")
	}
	if result.OK {
		t.Errorf("expected ok=false")
	}
	if result.Error == nil || !strings.Contains(*result.Error, "push-only") {
		t.Errorf("error must name the push-only refusal (either the tag-existence precondition or the --at incompatibility, whichever fires first); got %s", *result.Error)
	}
}

// =============================================================================
// SUPERSEDED CEREMONY — tagless refusal (immutability cannot mask it)
// =============================================================================

// TestReleaseTag_Historical_SupersededCeremony_AtOldM_Refuses_Tagless —
// after a re-ceremony (M2 superseding M1), --at <M1> must refuse BECAUSE
// the current committed anchors select M2. The fixture is deliberately
// TAGLESS so the published-tag immutability refusal cannot mask the
// eligibility refusal (immutability would also refuse, for the wrong
// reason, if the old tag existed).
func TestReleaseTag_Historical_SupersededCeremony_AtOldM_Refuses_Tagless(t *testing.T) {
	fx := setupHistoricalFixture(t, historicalFixtureOpts{})
	m1 := fx.msha
	msgFile := histMsgFile(t, "v0.2.0")

	// Re-ceremony on top of H1: note2 (changed content) -> artifact2 ->
	// manifest2 (M2) -> chore2 (H2).
	commitScratchFile(t, fx.scratch, "templates/migrations/v0.2.0.md", "# v0.2.0\n\nRe-ceremonied note content (supersedes M1).\n", "re-ceremony note (N2)")
	n2 := gitRevParseVerify(t, fx.scratch, "HEAD")
	artifact2 := buildReadinessArtifactBytes(t, readinessArtifactSpec{CommitSHA: n2})
	commitScratchFile(t, fx.scratch, ".vh-agent-harness/release-readiness-pass.json", string(artifact2), "re-ceremony artifact (R2)")
	restampManifest(t, fx.scratch, fx.spec)
	m2 := gitRevParseVerify(t, fx.scratch, "HEAD")
	commitScratchFile(t, fx.scratch, "fileB.go", "package scratch\n\n// ChoreAfterM2 marks the second chore.\nconst ChoreAfterM2 = true\n", "chore after M2")
	fx.hsha = gitRevParseVerify(t, fx.scratch, "HEAD")

	if tagExists(t, fx.scratch, "v0.2.0") {
		t.Fatalf("fixture must be tagless so immutability cannot mask the eligibility refusal")
	}

	exitCode, result, _, _ := runReleaseTagHistorical(t, fx.wrapper, fx.scratch, msgFile, "v0.2.0",
		[]string{"--at", m1}, nil, 60*time.Second)
	if exitCode == 0 {
		t.Fatalf("--at <superseded M1> must REFUSE (tagless); got exit 0")
	}
	if result.OK {
		t.Errorf("expected ok=false")
	}
	if result.Error == nil ||
		!strings.Contains(*result.Error, "--at assertion failed") ||
		!strings.Contains(*result.Error, m1) ||
		!strings.Contains(*result.Error, m2) ||
		!strings.Contains(*result.Error, "superseded") {
		t.Errorf("error must name M1, the eligible M2, and the superseded semantics; got %s", *result.Error)
	}
	if tagExists(t, fx.scratch, "v0.2.0") {
		t.Errorf("tag must NOT exist")
	}
	if result.Historical == nil || !*result.Historical {
		t.Errorf("historical must be true (M2 derived, H advanced past it); got %v", result.Historical)
	}
}

// =============================================================================
// DEFECT AT M, GREEN AT H — refuses in the verification worktree
// =============================================================================

// TestReleaseTag_Historical_BrokenCodeAtM_GreenAtH_Refuses — a compile
// defect that exists at M and is repaired at H still refuses: G0 evaluates
// M's tree inside the detached verification worktree, not H's.
func TestReleaseTag_Historical_BrokenCodeAtM_GreenAtH_Refuses(t *testing.T) {
	fx := setupHistoricalFixture(t, historicalFixtureOpts{
		BrokenGo:    "package scratch\n\nfunc broken( {}\n",
		FixBrokenGo: "package scratch\n\n// Fixed after M.\nconst FixedAfterM = true\n",
	})
	msgFile := histMsgFile(t, "v0.2.0")

	// Sanity: H is green in the ORIGINAL checkout (the repair landed) —
	// the classic-flow G0 would pass, which is exactly the hole the
	// historical isolation closes.
	if out, err := exec.Command("git", "-C", fx.scratch, "status", "--short").CombinedOutput(); err != nil || len(strings.TrimSpace(string(out))) != 0 {
		t.Fatalf("fixture must be clean at H: %v\n%s", err, out)
	}

	exitCode, result, _, _ := runReleaseTagHistorical(t, fx.wrapper, fx.scratch, msgFile, "v0.2.0", nil, nil, 120*time.Second)
	if exitCode == 0 {
		t.Fatalf("broken-at-M must REFUSE despite the green H; got exit 0")
	}
	if result.OK {
		t.Errorf("expected ok=false")
	}
	if result.Error == nil || !strings.Contains(*result.Error, "failed at M (verification worktree)") {
		t.Errorf("error must name the verification-worktree G0 refusal; got %s", *result.Error)
	}
	if result.Historical == nil || !*result.Historical {
		t.Errorf("historical must be true; got %v", result.Historical)
	}
	if tagExists(t, fx.scratch, "v0.2.0") {
		t.Errorf("tag must NOT exist")
	}
	// The EXIT-trap must have cleaned the worktree even on refusal.
	if got := worktreeEntryCount(t, fx.scratch); got != 1 {
		t.Errorf("worktree list must be back to 1 entry after refusal; got %d", got)
	}
	if leftovers := releaseVerifyLeftovers(t, fx); len(leftovers) != 0 {
		t.Errorf("verification worktree leaked after refusal: %v", leftovers)
	}
}

// =============================================================================
// DIRTY ORIGINAL — refuses despite a clean X
// =============================================================================

// TestReleaseTag_Historical_DirtyOriginal_PreIsolation_Refuses — a dirty
// ORIGINAL checkout refuses at the pre-isolation G0b, BEFORE any
// verification worktree is created: X being clean by construction cannot
// substitute for a clean original.
func TestReleaseTag_Historical_DirtyOriginal_PreIsolation_Refuses(t *testing.T) {
	fx := setupHistoricalFixture(t, historicalFixtureOpts{})
	msgFile := histMsgFile(t, "v0.2.0")

	// Uncommitted tracked edit on the original checkout.
	dirty := filepath.Join(fx.scratch, "fileA.go")
	if err := os.WriteFile(dirty, []byte("package scratch\n\n// dirty\nconst DirtyOriginal = true\n"), 0o644); err != nil {
		t.Fatalf("dirty fileA.go: %v", err)
	}

	exitCode, result, _, _ := runReleaseTagHistorical(t, fx.wrapper, fx.scratch, msgFile, "v0.2.0", nil, nil, 120*time.Second)
	if exitCode == 0 {
		t.Fatalf("dirty original must REFUSE; got exit 0")
	}
	if result.OK {
		t.Errorf("expected ok=false")
	}
	if result.Error == nil || !strings.Contains(*result.Error, "pre-isolation") {
		t.Errorf("error must name the pre-isolation G0b check; got %s", *result.Error)
	}
	// Refusal happens BEFORE worktree creation: nothing to sweep, nothing
	// registered, nothing on disk.
	if got := worktreeEntryCount(t, fx.scratch); got != 1 {
		t.Errorf("no verification worktree may be created on a pre-isolation refusal; got %d entries", got)
	}
	if leftovers := releaseVerifyLeftovers(t, fx); len(leftovers) != 0 {
		t.Errorf("no worktree directory may exist; got %v", leftovers)
	}
	if tagExists(t, fx.scratch, "v0.2.0") {
		t.Errorf("tag must NOT exist")
	}
}

// =============================================================================
// PROTECTED-PATH SCAN — edits AND reverts refuse
// =============================================================================

// TestReleaseTag_Historical_ProtectedPathEdit_Refuses — an intervening
// commit that edits a protected ceremony-surface path refuses.
func TestReleaseTag_Historical_ProtectedPathEdit_Refuses(t *testing.T) {
	// The chore edits the wrapper itself (append an inert trailing comment
	// so the running wrapper copy is unaffected semantically).
	fx := setupHistoricalFixture(t, historicalFixtureOpts{})
	wrapperBody, err := os.ReadFile(fx.wrapper)
	if err != nil {
		t.Fatalf("read wrapper: %v", err)
	}
	msgFile := histMsgFile(t, "v0.2.0")

	commitScratchFile(t, fx.scratch, "scripts/release-tag.sh", string(wrapperBody)+"\n# chore: append an inert trailing comment\n", "chore touching the wrapper")
	fx.hsha = gitRevParseVerify(t, fx.scratch, "HEAD")

	exitCode, result, _, _ := runReleaseTagHistorical(t, fx.wrapper, fx.scratch, msgFile, "v0.2.0", nil, nil, 60*time.Second)
	if exitCode == 0 {
		t.Fatalf("protected-path edit must REFUSE; got exit 0")
	}
	if result.OK {
		t.Errorf("expected ok=false")
	}
	if result.Error == nil ||
		!strings.Contains(*result.Error, "protected-path change in intervening commit") ||
		!strings.Contains(*result.Error, "scripts/release-tag.sh") {
		t.Errorf("error must name the protected path and the intervening commit; got %s", *result.Error)
	}
	if tagExists(t, fx.scratch, "v0.2.0") {
		t.Errorf("tag must NOT exist")
	}
}

// TestReleaseTag_Historical_ProtectedPathRevert_Refuses — a revert also
// counts: a pre-ceremony commit edits the DEFER evaluator, and the chore
// restores the original bytes. The chore's diff against its parent touches
// the protected path, so the scan refuses even though M..H ends up with
// byte-identical content at H (the net-content equality must NOT mute the
// intervening-commit scan — that is exactly the edit/revert laundering the
// scan exists to catch).
func TestReleaseTag_Historical_ProtectedPathRevert_Refuses(t *testing.T) {
	// Recompute the rendered evaluator bytes the fixture installed, so the
	// chore can restore them exactly.
	root := findModuleRoot(t)
	evalBody, err := os.ReadFile(filepath.Join(root, "templates", "core", ".opencode", "scripts", "check-defer-triggers.mjs"))
	if err != nil {
		t.Fatalf("read evaluator template: %v", err)
	}
	originalEval := strings.ReplaceAll(string(evalBody), "{{COORDINATOR_DIR}}", "coordinator")
	editedEval := originalEval + "\n// pre-ceremony edit (to be reverted by the chore)\n"

	fx := setupHistoricalFixture(t, historicalFixtureOpts{
		PreNoteRel:     ".opencode/scripts/check-defer-triggers.mjs",
		PreNoteContent: editedEval,
		ChoreRel:       ".opencode/scripts/check-defer-triggers.mjs",
		ChoreContent:   originalEval,
	})
	msgFile := histMsgFile(t, "v0.2.0")

	// Net content at H is byte-identical to the pre-edit state — the scan
	// must still refuse on the intervening revert.
	exitCode, result, _, _ := runReleaseTagHistorical(t, fx.wrapper, fx.scratch, msgFile, "v0.2.0", nil, nil, 60*time.Second)
	if exitCode == 0 {
		t.Fatalf("protected-path revert must REFUSE; got exit 0")
	}
	if result.OK {
		t.Errorf("expected ok=false")
	}
	if result.Error == nil ||
		!strings.Contains(*result.Error, "protected-path change in intervening commit") ||
		!strings.Contains(*result.Error, ".opencode/scripts/check-defer-triggers.mjs") {
		t.Errorf("error must name the protected path and the intervening commit; got %s", *result.Error)
	}
	if tagExists(t, fx.scratch, "v0.2.0") {
		t.Errorf("tag must NOT exist")
	}
}

// TestReleaseTag_Historical_InterveningLineageDeletion_Refuses — the round-2
// G0c activation-bypass blocker (real-binary arm): lineage.yml was in NEITHER
// the protected-path scan NOR the byte-equality set, so a post-M commit
// DELETING it passed every eligibility check while making the single H-side
// `[ -f .vh-agent-harness/lineage.yml ]` gate switch off BOTH doctor runs
// (control AND historical target) — a target-class-defective M could be
// tagged with no health verdict at all. Post-fix the deletion is a
// protected-path hit (git diff --name-only lists deleted paths), so
// eligibility refuses before any doctor applicability is derived. RED
// receipt: on the pre-fix tree this arm fails with the run ACCEPTING — both
// doctor runs skipped, tag cut at M.
//
// Per-side gating note: post-fix, the "lineage.yml exists at M but not in
// H's worktree while the target run still executes" variant is NOT
// constructible at the wrapper level — H's worktree state is H's committed
// state (G0b refuses dirt), so the only way to diverge M-blob from H-file is
// an intervening M..H change, which the protected-path scan now refuses
// before G0c. The per-side gates remain the structural defense (each side
// derives applicability from its own tree, never H's), but the scan is what
// is observable here.
func TestReleaseTag_Historical_InterveningLineageDeletion_Refuses(t *testing.T) {
	// Real-binary fixture: the ceremony binary is present and the full seam
	// render gives both doctor sides a real harness install (pre-fix the run
	// skipped both doctor runs and accepted regardless).
	fx := setupHistoricalSeamRenderFixture(t, nil)
	copyHarnessBinaryToCeremony(t, fx.scratch)
	// The chore DELETES the lineage record — the G0c seam-managed marker.
	removeScratchFile(t, fx.scratch, ".vh-agent-harness/lineage.yml", "chore: drop lineage after M")
	fx.hsha = gitRevParseVerify(t, fx.scratch, "HEAD")

	msgFile := histMsgFile(t, "v0.2.0")
	exitCode, result, _, _ := runReleaseTagHistorical(t, fx.wrapper, fx.scratch, msgFile, "v0.2.0", nil, nil, 300*time.Second)
	if exitCode == 0 {
		t.Fatalf("intervening lineage.yml deletion must REFUSE (protected-path hit); got exit 0 — the pre-fix activation-bypass shape (both doctor runs skipped, tag accepted)")
	}
	if result.OK {
		t.Errorf("expected ok=false")
	}
	if result.Error == nil ||
		!strings.Contains(*result.Error, "protected-path change in intervening commit") ||
		!strings.Contains(*result.Error, ".vh-agent-harness/lineage.yml") {
		t.Errorf("error must name the lineage path and the intervening commit; got %s", *result.Error)
	}
	if result.Historical == nil || !*result.Historical {
		t.Errorf("historical must be true; got %v", result.Historical)
	}
	if tagExists(t, fx.scratch, "v0.2.0") {
		t.Errorf("tag must NOT exist")
	}
	requireCleanHistoricalRepo(t, fx)
	// The eligibility refusal precedes isolation: no worktree may exist.
	if got := worktreeEntryCount(t, fx.scratch); got != 1 {
		t.Errorf("no verification worktree may be created on the eligibility refusal; got %d entries", got)
	}
	if leftovers := releaseVerifyLeftovers(t, fx); len(leftovers) != 0 {
		t.Errorf("no verification worktree directory may exist; got %v", leftovers)
	}
}

// TestReleaseTag_Historical_ProtectedPathRenameOut_Refuses — the round-6 F7
// rename-based eligibility bypass: with git's default rename detection,
// `git diff --name-only` reports a pure rename as ONLY the (unprotected)
// destination, omitting the source — so a chore RENAMING the lineage record
// out of the ceremony surface (.vh-agent-harness/lineage.yml ->
// docs/lineage.yml) evaded the protected-path scan while flipping G0c
// applicability exactly like the deletion arm above. RED receipt: against
// the pre-fix wrapper (round-5 scan, rename-detecting diff) this arm fails
// with the run ACCEPTING — the tag is cut at M — despite the premise pin
// proving the rename-detecting diff hides the source path. Post-fix the scan
// diffs with --no-renames: the source appears as a deletion and eligibility
// refuses naming the protected path.
func TestReleaseTag_Historical_ProtectedPathRenameOut_Refuses(t *testing.T) {
	// Real-binary seam-render fixture (the plain hand-written-lineage shape
	// would refuse at G0c for structural-unhealthiness reasons unrelated to
	// the scan): committed seam render below the ceremony, real ceremony
	// binary, rename chore above the default benign chore.
	fx := setupHistoricalSeamRenderFixture(t, nil)
	copyHarnessBinaryToCeremony(t, fx.scratch)

	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", fx.scratch}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	if err := os.MkdirAll(filepath.Join(fx.scratch, "docs"), 0o755); err != nil {
		t.Fatalf("mkdir docs: %v", err)
	}
	git("mv", ".vh-agent-harness/lineage.yml", "docs/lineage.yml")
	git("commit", "-q", "-m", "chore: rename the lineage record out of the ceremony surface")
	fx.hsha = gitRevParseVerify(t, fx.scratch, "HEAD")

	// PREMISE PIN — the exact two-sided blindness the --no-renames fix
	// closes. Without it the refusal below could rest on a fixture whose
	// rename was not rename-detectable at git defaults.
	choreParent := gitRevParseVerify(t, fx.scratch, fx.hsha+"^")
	def := gitDiffNameOnly(t, fx.scratch, choreParent, fx.hsha)
	if def[".vh-agent-harness/lineage.yml"] {
		t.Fatalf("premise: rename-detecting diff must OMIT the rename source (git defaults changed?)")
	}
	if !def["docs/lineage.yml"] {
		t.Fatalf("premise: rename-detecting diff must show the destination; got %v", def)
	}
	noR := gitDiffNameOnlyEx(t, fx.scratch, []string{"--no-renames"}, choreParent, fx.hsha)
	if !noR[".vh-agent-harness/lineage.yml"] || !noR["docs/lineage.yml"] {
		t.Fatalf("premise: --no-renames must list source AND destination; got %v", noR)
	}

	msgFile := histMsgFile(t, "v0.2.0")
	exitCode, result, _, _ := runReleaseTagHistorical(t, fx.wrapper, fx.scratch, msgFile, "v0.2.0", nil, nil, 300*time.Second)
	if exitCode == 0 {
		tagPeel := "(no tag)"
		if tagExists(t, fx.scratch, "v0.2.0") {
			tagPeel = gitRevParseVerify(t, fx.scratch, "refs/tags/v0.2.0^{commit}")
		}
		t.Fatalf("rename OUT of a protected path must REFUSE; got exit 0 (pre-fix rename-bypass shape: tag=%s)", tagPeel)
	}
	if result.OK {
		t.Errorf("expected ok=false")
	}
	if result.Error == nil ||
		!strings.Contains(*result.Error, "protected-path change in intervening commit") ||
		!strings.Contains(*result.Error, ".vh-agent-harness/lineage.yml") {
		got := "(nil)"
		if result.Error != nil {
			got = *result.Error
		}
		t.Errorf("error must name the renamed protected path and the intervening commit; got %s", got)
	}
	if result.Historical == nil || !*result.Historical {
		t.Errorf("historical must be true; got %v", result.Historical)
	}
	if tagExists(t, fx.scratch, "v0.2.0") {
		t.Errorf("tag must NOT exist")
	}
	// The eligibility refusal precedes isolation: no worktree may exist.
	if got := worktreeEntryCount(t, fx.scratch); got != 1 {
		t.Errorf("no verification worktree may be created on the eligibility refusal; got %d entries", got)
	}
	if leftovers := releaseVerifyLeftovers(t, fx); len(leftovers) != 0 {
		t.Errorf("no verification worktree directory may exist; got %v", leftovers)
	}
	requireCleanHistoricalRepo(t, fx)
}

// =============================================================================
// SUFFIX MERGES — refused in v1
// =============================================================================

// TestReleaseTag_Historical_SuffixMerge_Refuses — a merge commit on H's
// first-parent chain above M refuses (v1: uninterrupted linear ceremony
// continuation only).
func TestReleaseTag_Historical_SuffixMerge_Refuses(t *testing.T) {
	fx := setupHistoricalFixture(t, historicalFixtureOpts{NoChore: true})
	msgFile := histMsgFile(t, "v0.2.0")

	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", fx.scratch}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git("branch", "side")
	git("checkout", "-q", "side")
	commitScratchFile(t, fx.scratch, "fileD.go", "package scratch\n\n// SideBranch marks the merged side branch.\nconst SideBranch = true\n", "side branch work")
	git("checkout", "-q", fx.branch)
	git("merge", "--no-ff", "-q", "-m", "merge side branch above M", "side")
	fx.hsha = gitRevParseVerify(t, fx.scratch, "HEAD")

	exitCode, result, _, _ := runReleaseTagHistorical(t, fx.wrapper, fx.scratch, msgFile, "v0.2.0", nil, nil, 60*time.Second)
	if exitCode == 0 {
		t.Fatalf("suffix merge must REFUSE; got exit 0")
	}
	if result.OK {
		t.Errorf("expected ok=false")
	}
	if result.Error == nil || !strings.Contains(*result.Error, "suffix merge") {
		t.Errorf("error must name the suffix-merge refusal; got %s", *result.Error)
	}
	if tagExists(t, fx.scratch, "v0.2.0") {
		t.Errorf("tag must NOT exist")
	}
}

// =============================================================================
// LIFECYCLE — stale sweep, unrelated worktrees, cleanup failure, drift
// =============================================================================

// TestReleaseTag_Historical_StaleWorktreeSwept_UnrelatedPreserved — a
// registered stale verification worktree for THIS version is swept before
// a fresh one is created (SIGKILL recovery), while unrelated worktrees
// (different prefix, or outside tmp/) are preserved untouched.
func TestReleaseTag_Historical_StaleWorktreeSwept_UnrelatedPreserved(t *testing.T) {
	fx := setupHistoricalFixture(t, historicalFixtureOpts{})
	msgFile := histMsgFile(t, "v0.2.0")

	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", fx.scratch}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	stale := filepath.Join(fx.scratch, "tmp", "release-verify-v0.2.0-stale1234")
	keepTmp := filepath.Join(fx.scratch, "tmp", "keep-other")
	// "Outside tmp/" but under the already-ignored bin/ so the fixture's
	// pre-isolation G0b still sees a clean tree (the assertion target is
	// worktree REGISTRATION preservation, not gitignore visibility).
	keepOutside := filepath.Join(fx.scratch, "bin", "wt-outside")
	git("worktree", "add", "--detach", stale, fx.msha)
	git("worktree", "add", "--detach", keepTmp, fx.hsha)
	git("worktree", "add", "--detach", keepOutside, fx.hsha)
	t.Cleanup(func() {
		for _, wt := range []string{keepTmp, keepOutside} {
			if _, err := os.Stat(wt); err == nil {
				_ = exec.Command("git", "-C", fx.scratch, "worktree", "remove", "--force", wt).Run()
			}
		}
	})

	exitCode, result, _, _ := runReleaseTagHistorical(t, fx.wrapper, fx.scratch, msgFile, "v0.2.0", nil, nil, 120*time.Second)
	if exitCode != 0 || !result.OK {
		errText := "<nil>"
		if result.Error != nil {
			errText = *result.Error
		}
		t.Fatalf("stale sweep must not block an eligible run; exit=%d ok=%v error=%s", exitCode, result.OK, errText)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Errorf("stale verification worktree must be swept; stat err=%v", err)
	}
	if _, err := os.Stat(keepTmp); err != nil {
		t.Errorf("unrelated tmp/ worktree must be preserved: %v", err)
	}
	if _, err := os.Stat(keepOutside); err != nil {
		t.Errorf("worktree outside tmp/ must be preserved: %v", err)
	}
	if !tagExists(t, fx.scratch, "v0.2.0") {
		t.Errorf("tag must exist")
	}
	requireCleanHistoricalRepo(t, fx)
}

// TestReleaseTag_Historical_CleanupFailure_Refuses — when the pre-tag
// cleanup cannot remove the verification worktree, the release REFUSES (a
// leaked worktree must never be papered over by cutting the tag anyway).
// The failure is induced by chmod 0555-ing the original repo's tmp/ from
// inside the wrapper's own G0 run (the env-gated hooks_test.go module).
func TestReleaseTag_Historical_CleanupFailure_Refuses(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root: chmod 0555 does not block removal for root")
	}
	fx := setupHistoricalFixture(t, historicalFixtureOpts{Hooks: true})
	msgFile := histMsgFile(t, "v0.2.0")

	t.Cleanup(func() {
		// The hook leaves X mode 0555 — restore writability BEFORE any
		// removal (including t.TempDir's own RemoveAll, which runs after
		// this LIFO-registered cleanup).
		matches, _ := filepath.Glob(filepath.Join(fx.scratch, "tmp", "release-verify-*"))
		for _, m := range matches {
			_ = os.Chmod(m, 0o755)
		}
		_ = exec.Command("git", "-C", fx.scratch, "worktree", "prune").Run()
		for _, m := range matches {
			_ = exec.Command("git", "-C", fx.scratch, "worktree", "remove", "--force", m).Run()
			_ = os.RemoveAll(m)
		}
		_ = exec.Command("git", "-C", fx.scratch, "worktree", "prune").Run()
	})

	exitCode, result, _, _ := runReleaseTagHistorical(t, fx.wrapper, fx.scratch, msgFile, "v0.2.0", nil,
		map[string]string{"SCRATCH_HIST_LOCK_TMP": "1"}, 180*time.Second)
	if exitCode == 0 {
		t.Fatalf("cleanup failure must REFUSE; got exit 0")
	}
	if result.OK {
		t.Errorf("expected ok=false")
	}
	if result.Error == nil ||
		!strings.Contains(*result.Error, "cleanup of the verification worktree") ||
		!strings.Contains(*result.Error, "FAILED") {
		t.Errorf("error must name the cleanup failure and the manual remedy; got %s", *result.Error)
	}
	if tagExists(t, fx.scratch, "v0.2.0") {
		t.Errorf("tag must NOT exist on cleanup failure")
	}
}

// TestReleaseTag_Historical_HeadMovedDuringWindow_Refuses — in the
// historical flow, HEAD advancing past the H captured at start REFUSES
// (eligibility was derived against that H; a newer commit has not been
// scanned). The concurrent commit is planted by the env-gated hook firing
// inside the wrapper's own G0 run.
func TestReleaseTag_Historical_HeadMovedDuringWindow_Refuses(t *testing.T) {
	fx := setupHistoricalFixture(t, historicalFixtureOpts{Hooks: true})
	msgFile := histMsgFile(t, "v0.2.0")

	exitCode, result, _, _ := runReleaseTagHistorical(t, fx.wrapper, fx.scratch, msgFile, "v0.2.0", nil,
		map[string]string{"SCRATCH_HIST_MOVE_HEAD": "1"}, 180*time.Second)
	if exitCode == 0 {
		t.Fatalf("mid-window HEAD movement must REFUSE in the historical flow; got exit 0")
	}
	if result.OK {
		t.Errorf("expected ok=false")
	}
	if result.Error == nil || !strings.Contains(*result.Error, "final drift check: HEAD moved") {
		t.Errorf("error must name the historical final drift refusal; got %s", *result.Error)
	}
	if tagExists(t, fx.scratch, "v0.2.0") {
		t.Errorf("tag must NOT exist")
	}
	// The EXIT trap still cleaned the verification worktree.
	if got := worktreeEntryCount(t, fx.scratch); got != 1 {
		t.Errorf("worktree must be cleaned by the trap; got %d entries", got)
	}
}

// f8Anchor is the SINGLE insertion anchor for the F8 instrumented runner:
// the historical flow's PRETAG_HEAD initialization line, immediately before
// the historical HEAD re-compare read. Exactly one occurrence is required —
// no line-number and no alternate-anchor fallback.
const f8Anchor = "\n  PRETAG_HEAD=\"\"\n"

// f8Insertion is the fixed test-owned block spliced into the F8 instrumented
// runner immediately before the historical HEAD re-compare read. Fixed
// commands ONLY — no env-supplied input, no eval, no sleeps, no polling. It
// verifies the interval's preconditions (verification worktree already
// cleaned up; original worktree clean, i.e. the final G0b re-check already
// ran) BEFORE landing one real synchronous commit — exactly a concurrent
// commit arriving in the interval between the final gates and the pre-tag
// re-compare. Diagnostics go to STDERR only, so the wrapper's one-JSON-object
// stdout contract is untouched; under the wrapper's `set -euo pipefail` a
// failed git mutation aborts the runner immediately.
const f8Insertion = `  # --- F8 TEST INSTRUMENTATION (release_tag_historical_test.go) ---
  # Fixed test-owned commands only; never part of production.
  if [ -n "$(git status --short 2>&1)" ]; then
    printf 'f8-instrumentation: original worktree NOT clean at the pre-tag interval (final G0b re-check not yet run)\n' >&2
    exit 9
  fi
  if git worktree list --porcelain 2>/dev/null | grep -Fqx "worktree ${X_DIR}"; then
    printf 'f8-instrumentation: verification worktree still registered at the pre-tag interval (cleanup not yet run)\n' >&2
    exit 9
  fi
  if ls "${REPO_ROOT}/tmp" 2>/dev/null | grep -q '^release-verify-'; then
    printf 'f8-instrumentation: verification worktree scratch still present at the pre-tag interval (cleanup not yet run)\n' >&2
    exit 9
  fi
  printf 'package scratch\n\n// IntervalCommit landed between the final gates and the pre-tag re-compare.\nconst IntervalCommit = true\n' > fileE.go
  git add fileE.go
  git commit -q -m 'concurrent commit in the pre-tag interval (F8 test instrumentation)'
`

// gitForEachRef lists every ref name in the repo (the exact-ref-delta pin of
// the anti-laundering arm).
func gitForEachRef(t *testing.T, repo string) []string {
	t.Helper()
	var refs []string
	for _, ln := range strings.Split(gitOut(t, repo, "for-each-ref", "--format=%(refname)"), "\n") {
		if r := strings.TrimSpace(ln); r != "" {
			refs = append(refs, r)
		}
	}
	return refs
}

// TestReleaseTag_Historical_HeadMovedInPreTagInterval_Refuses — the round-6/7
// F8 pre-tag HEAD-drift window: the final drift check runs BEFORE worktree
// cleanup and the final G0b re-check, leaving an interval immediately before
// `git tag` in which a concurrent commit can advance HEAD past the H the
// eligibility scan certified — the wrapper would then tag a superseded
// window. The round-6 fix is the pre-tag HEAD re-compare; this round removes
// the production test seam that change rode in on, so the arm now drives the
// interval with NO wrapper cooperation at all:
//
//   - the production source is read and required to be byte-identical to the
//     committed pristine fixture wrapper (fail closed on drift);
//   - a throwaway INSTRUMENTED RUNNER is derived under the scratch repo's
//     ignored tmp/ by splicing f8Insertion at the single f8Anchor —
//     immediately before the historical HEAD read; the committed fixture
//     wrapper itself is NEVER modified (that would dirty G0b and launder the
//     instrumentation), and the reverse-splice pin proves the runner is
//     exactly source+insertion;
//   - the spliced block verifies the interval preconditions (cleanup done,
//     worktree clean) and lands one real synchronous commit; the wrapper then
//     executes with its original scratch cwd.
//
// Placement is pinned TWICE, because splice byte-equality alone proves
// nothing about where the insertion sits:
//
//   - structurally: exact-block offsets in the source must obey
//     cleanup < final-G0b < anchor < re-compare < tag-mutation (narrow
//     exact-block pins, not a general shell parser);
//   - at runtime: the spliced preconditions abort the runner (exit 9) if the
//     interval's gates have not actually run when the anchor is reached.
func TestReleaseTag_Historical_HeadMovedInPreTagInterval_Refuses(t *testing.T) {
	fx := setupHistoricalFixture(t, historicalFixtureOpts{})
	msgFile := histMsgFile(t, "v0.2.0")

	// --- source + pristine-copy pins -------------------------------------
	root := findModuleRoot(t)
	srcBytes, err := os.ReadFile(filepath.Join(root, "scripts", "release-tag.sh"))
	if err != nil {
		t.Fatalf("read production wrapper source: %v", err)
	}
	pristineBytes, err := os.ReadFile(fx.wrapper)
	if err != nil {
		t.Fatalf("read pristine fixture wrapper: %v", err)
	}
	if string(srcBytes) != string(pristineBytes) {
		t.Fatalf("fixture wrapper must be byte-identical to the production source (fail closed on drift — instrumenting a diverged wrapper would test the wrong code)")
	}
	src := string(srcBytes)

	// --- structural ordering pins (narrow exact blocks, no parser) -------
	blocks := []struct{ name, needle string }{
		{"pre-tag worktree cleanup", "if ! cleanup_verify_worktree; then"},
		{"final G0b re-check", "G0B2_OUTPUT=$(git status --short 2>&1) || true"},
		{"single insertion anchor", f8Anchor},
		{"historical HEAD re-compare", "if ! PRETAG_HEAD=$(git rev-parse HEAD 2>/dev/null) || [ \"$PRETAG_HEAD\" != \"$H_SHA\" ]; then"},
		{"M-pinned tag mutation", "if ! TAG_ERR=$(git tag -a -F \"$MSG_FILE\" \"$VERSION\" \"$TAG_TARGET\" 2>&1 1>/dev/null); then"},
	}
	offsets := make([]int, len(blocks))
	for i, b := range blocks {
		if n := strings.Count(src, b.needle); n != 1 {
			t.Fatalf("structural pin %q must appear EXACTLY once in the production source (got %d) — refusing to instrument a wrapper whose shape drifted", b.name, n)
		}
		offsets[i] = strings.Index(src, b.needle)
	}
	for i := 0; i < len(blocks)-1; i++ {
		if offsets[i] >= offsets[i+1] {
			t.Fatalf("structural order violated: %q (offset %d) must precede %q (offset %d)", blocks[i].name, offsets[i], blocks[i+1].name, offsets[i+1])
		}
	}

	// --- instrumented runner under the scratch repo's ignored tmp/ -------
	insertAt := offsets[2] + len(f8Anchor) // immediately before the historical HEAD read
	runner := src[:insertAt] + f8Insertion + src[insertAt:]
	// Reverse-splice equality: removing EXACTLY the recorded insertion span
	// must reproduce the source bytes — the runner is source + insertion and
	// nothing else.
	if runner[:insertAt]+runner[insertAt+len(f8Insertion):] != src {
		t.Fatalf("reverse-splice pin failed: the instrumented runner is not exactly source+insertion")
	}
	runnerPath := filepath.Join(fx.scratch, "tmp", "release-tag-f8-instrumented.sh")
	if err := os.WriteFile(runnerPath, []byte(runner), 0o755); err != nil {
		t.Fatalf("write instrumented runner: %v", err)
	}

	tagsBefore := gitOut(t, fx.scratch, "tag", "-l")
	exitCode, result, _, stderr := runReleaseTagHistorical(t, runnerPath, fx.scratch, msgFile, "v0.2.0", nil, nil, 180*time.Second)
	if exitCode == 9 {
		t.Fatalf("F8 instrumentation aborted at its interval preconditions — the runner reached the anchor before cleanup/final-G0b ran (ordering regression):\n%s", stderr)
	}
	if exitCode == 0 {
		tagPeel := "(no tag)"
		if tagExists(t, fx.scratch, "v0.2.0") {
			tagPeel = gitRevParseVerify(t, fx.scratch, "refs/tags/v0.2.0^{commit}")
		}
		headNow := gitRevParseVerify(t, fx.scratch, "HEAD")
		t.Fatalf("HEAD advance in the pre-tag interval must REFUSE; got exit 0 (window shape: tag=%s, HEAD=%s vs H=%s)", tagPeel, headNow, fx.hsha)
	}
	if result.OK {
		t.Errorf("expected ok=false")
	}
	if result.Error == nil ||
		!strings.Contains(*result.Error, "historical-M pre-tag drift check") ||
		!strings.Contains(*result.Error, "HEAD moved between the final drift check and the tag mutation") {
		got := "(nil)"
		if result.Error != nil {
			got = *result.Error
		}
		t.Errorf("refusal must be the exact historical pre-tag drift category (not merely a nonzero exit); got %s", got)
	}
	if result.Historical == nil || !*result.Historical {
		t.Errorf("historical must be true; got %v", result.Historical)
	}
	// The insertion actually ran: HEAD sits exactly one commit above H and
	// that commit's parent is exactly H (an unmoved HEAD means the splice
	// never executed and the refusal above would be vacuous).
	headNow := gitRevParseVerify(t, fx.scratch, "HEAD")
	if headNow == fx.hsha {
		t.Fatalf("premise: the inserted commit must advance HEAD past H (%s); HEAD unchanged — the insertion did not run", fx.hsha)
	}
	if parent := gitRevParseVerify(t, fx.scratch, headNow+"^"); parent != fx.hsha {
		t.Errorf("the concurrent commit's parent must be exactly H (%s); got %s", fx.hsha, parent)
	}
	if got := gitOut(t, fx.scratch, "rev-list", "--count", fx.hsha+"..HEAD"); got != "1" {
		t.Errorf("exactly one inserted commit may sit above H; got %s", got)
	}
	// No release tag and no unexpected tag at all: the tag set is unchanged.
	if after := gitOut(t, fx.scratch, "tag", "-l"); after != tagsBefore {
		t.Errorf("tag set must be unchanged (no v0.2.0, nothing added);\nbefore:\n%s\nafter:\n%s", tagsBefore, after)
	}
	// Cleanup already ran before the interval: no worktree may remain.
	if got := worktreeEntryCount(t, fx.scratch); got != 1 {
		t.Errorf("worktree list must be back to exactly 1 entry; got %d", got)
	}
	if leftovers := releaseVerifyLeftovers(t, fx); len(leftovers) != 0 {
		t.Errorf("no verification worktree directory may exist; got %v", leftovers)
	}
	// The inserted commit committed its write: the scratch must be clean.
	if st := gitOut(t, fx.scratch, "status", "--short"); st != "" {
		t.Errorf("scratch must be clean after the refused run (the insertion committed its write);\n%s", st)
	}
}

// pretagHookHostilePayload is the ONE fixed hostile payload used identically
// by the positive control and the production case of the anti-laundering arm:
// exactly the command string the removed RELEASE_TAG_TEST_PRETAG_HOOK seam
// would have eval'd in the wrapper's pre-tag interval — an observable
// ignored-path sentinel, an unauthorized tag, and a raw commit.
const pretagHookHostilePayload = `printf 'hook-fired' > tmp/pretag-hook-sentinel; git tag v9.9.9-hostile; printf 'package scratch\n\n// HostileHookCommit must never land via a wrapper seam.\nconst HostileHookCommit = true\n' > fileE.go; git add fileE.go; git commit -q -m 'hostile commit (old-hook-channel anti-laundering arm)'`

// TestReleaseTag_Historical_OldHookChannelRemoved — anti-laundering pin over
// the round-7 removal of the round-6 test seam: the production wrapper must
// carry ZERO trace of the RELEASE_TAG_TEST_PRETAG_HOOK channel (the identifier
// itself or any renamed/indirect replacement), and a hostile value for that
// exact identifier — supplied unfiltered in the runner's environment against
// the PRISTINE committed wrapper — must be completely inert: a normal
// historical SUCCESS with the exact permitted ref delta.
//
// POSITIVE CONTROL FIRST: the exact payload runs in a separate disposable
// fixture with equivalent Bash/cwd/path semantics (bash -c, cwd = scratch
// root, same git identity and ignore shape) and must succeed with observable
// effects — proving the production case's quiet is the removed channel, not a
// broken payload. (`bash -c` vs the old `eval` differ only in shell process,
// not in any observable effect for these fixed commands.)
//
// SECURITY CLAIM BOUNDARY: this arm proves the IDENTIFIED hook channel
// (RELEASE_TAG_TEST_PRETAG_HOOK eval in the historical pre-tag interval) is
// removed from production. It does NOT prove universal env-var safety — the
// wrapper legitimately reads other environment variables
// (RELEASE_TAG_MESSAGE_FILE, RELEASE_TAG_PUSH), and no claim is made about
// any channel other than the removed seam. Do not relabel this arm broader.
func TestReleaseTag_Historical_OldHookChannelRemoved(t *testing.T) {
	// --- positive control -------------------------------------------------
	fxCtl := setupHistoricalFixture(t, historicalFixtureOpts{})
	ctl := exec.Command("bash", "-c", pretagHookHostilePayload)
	ctl.Dir = fxCtl.scratch
	if out, err := ctl.CombinedOutput(); err != nil {
		t.Fatalf("positive control: the hostile payload must execute cleanly with scratch cwd semantics (otherwise the production case is vacuous):\n%v\n%s", err, out)
	}
	ctlSentinel := filepath.Join(fxCtl.scratch, "tmp", "pretag-hook-sentinel")
	if b, err := os.ReadFile(ctlSentinel); err != nil || string(b) != "hook-fired" {
		t.Fatalf("positive control: the sentinel must exist with content \"hook-fired\" (err=%v body=%q)", err, string(b))
	}
	if !tagExists(t, fxCtl.scratch, "v9.9.9-hostile") {
		t.Fatalf("positive control: the unauthorized tag must have been created")
	}
	if got := gitOut(t, fxCtl.scratch, "rev-list", "--count", fxCtl.hsha+"..HEAD"); got != "1" {
		t.Fatalf("positive control: the hostile commit must land exactly one commit above H; got %s", got)
	}

	// --- production case: pristine wrapper + hostile env, unfiltered ------
	fx := setupHistoricalFixture(t, historicalFixtureOpts{})
	msgFile := histMsgFile(t, "v0.2.0")

	// The pristine committed wrapper must be the production source.
	root := findModuleRoot(t)
	srcBytes, err := os.ReadFile(filepath.Join(root, "scripts", "release-tag.sh"))
	if err != nil {
		t.Fatalf("read production wrapper source: %v", err)
	}
	pristineBytes, err := os.ReadFile(fx.wrapper)
	if err != nil {
		t.Fatalf("read pristine fixture wrapper: %v", err)
	}
	if string(srcBytes) != string(pristineBytes) {
		t.Fatalf("fixture wrapper must be byte-identical to the production source")
	}

	headBefore := gitRevParseVerify(t, fx.scratch, "HEAD")
	symBefore := gitOut(t, fx.scratch, "symbolic-ref", "--short", "HEAD")
	branchRefBefore := gitRevParseVerify(t, fx.scratch, "refs/heads/"+fx.branch)
	indexBefore := gitOut(t, fx.scratch, "ls-files", "-s")
	refsBefore := gitForEachRef(t, fx.scratch)

	// Environment construction is explicit here (not inside the shared
	// helper) so the arm can PROVE the hostile channel is supplied unfiltered
	// to the runner: any pre-existing entry for the identifier is stripped
	// and exactly one — the hostile one — is appended and verified present
	// before spawn. A helper regression that dropped extraEnv would otherwise
	// make this arm vacuously green.
	hookEntry := "RELEASE_TAG_TEST_PRETAG_HOOK=" + pretagHookHostilePayload
	env := make([]string, 0, len(os.Environ())+2)
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, "RELEASE_TAG_TEST_PRETAG_HOOK=") {
			continue
		}
		env = append(env, kv)
	}
	env = append(env, "RELEASE_TAG_MESSAGE_FILE="+msgFile)
	env = append(env, hookEntry)
	supplied := 0
	for _, kv := range env {
		if kv == hookEntry {
			supplied++
		}
	}
	if supplied != 1 {
		t.Fatalf("runner env must carry the hostile hook entry EXACTLY once, unfiltered (got %d)", supplied)
	}

	cmd := exec.Command("bash", fx.wrapper, "v0.2.0")
	cmd.Dir = fx.scratch
	cmd.Env = env
	var outb, errb strings.Builder
	cmd.Stdout = &outb
	cmd.Stderr = &errb
	timer := time.AfterFunc(180*time.Second, func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
	})
	defer timer.Stop()
	runErr := cmd.Run()
	exitCode := 0
	if runErr != nil {
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			t.Fatalf("bash spawn error: %v\nstderr: %s", runErr, errb.String())
		}
	}
	stdout, stderr := outb.String(), errb.String()
	var result releaseTagHistoricalResult
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("wrapper output must be valid JSON (exit=%d): %v\nstdout:\n%s\nstderr:\n%s", exitCode, err, stdout, stderr)
	}
	if exitCode != 0 || !result.OK {
		t.Fatalf("pristine-wrapper historical run with the dead channel's env set must SUCCEED normally (the channel must be inert);\nexit=%d ok=%v\nstdout:\n%s\nstderr:\n%s", exitCode, result.OK, stdout, stderr)
	}
	if result.Historical == nil || !*result.Historical {
		t.Errorf("historical must be true; got %v", result.Historical)
	}
	if result.Commit == nil || *result.Commit != fx.msha {
		t.Errorf("commit must be the derived M %s; got %v", fx.msha, result.Commit)
	}
	if result.HeadDrift == nil || *result.HeadDrift {
		t.Errorf("head_drift must be false; got %v", result.HeadDrift)
	}
	if !tagExists(t, fx.scratch, "v0.2.0") {
		t.Fatalf("the intended release tag must exist")
	}
	if peeled := gitRevParseVerify(t, fx.scratch, "refs/tags/v0.2.0^{commit}"); peeled != fx.msha {
		t.Errorf("tag must peel to M %s; got %s", fx.msha, peeled)
	}
	// The hostile payload never executed: no sentinel, no hostile tag/commit.
	if _, err := os.Stat(filepath.Join(fx.scratch, "tmp", "pretag-hook-sentinel")); !os.IsNotExist(err) {
		t.Errorf("the hostile sentinel must NOT exist (the old seam would have written it); stat err=%v", err)
	}
	// EXACT permitted ref delta: only refs/tags/v0.2.0 added, nothing removed
	// or moved (no v9.9.9-hostile, no branch movement).
	refsAfter := gitForEachRef(t, fx.scratch)
	beforeSet := make(map[string]bool, len(refsBefore))
	for _, r := range refsBefore {
		beforeSet[r] = true
	}
	afterSet := make(map[string]bool, len(refsAfter))
	for _, r := range refsAfter {
		afterSet[r] = true
	}
	var added, removed []string
	for _, r := range refsAfter {
		if !beforeSet[r] {
			added = append(added, r)
		}
	}
	for _, r := range refsBefore {
		if !afterSet[r] {
			removed = append(removed, r)
		}
	}
	if len(added) != 1 || added[0] != "refs/tags/v0.2.0" || len(removed) != 0 {
		t.Errorf("ref delta must be EXACTLY {+refs/tags/v0.2.0}; added=%v removed=%v", added, removed)
	}
	// HEAD / symbolic branch / branch ref / semantic index unchanged.
	if got := gitRevParseVerify(t, fx.scratch, "HEAD"); got != headBefore {
		t.Errorf("HEAD must be unchanged (%s); got %s", headBefore, got)
	}
	if got := gitOut(t, fx.scratch, "symbolic-ref", "--short", "HEAD"); got != symBefore {
		t.Errorf("symbolic branch must be unchanged (%s); got %s", symBefore, got)
	}
	if got := gitRevParseVerify(t, fx.scratch, "refs/heads/"+fx.branch); got != branchRefBefore {
		t.Errorf("branch ref must be unchanged (%s); got %s", branchRefBefore, got)
	}
	if got := gitOut(t, fx.scratch, "ls-files", "-s"); got != indexBefore {
		t.Errorf("semantic index must be unchanged;\nbefore:\n%s\nafter:\n%s", indexBefore, got)
	}
	// Worktree clean; ceremony cleanup complete.
	if st := gitOut(t, fx.scratch, "status", "--short"); st != "" {
		t.Errorf("worktree must be clean;\n%s", st)
	}
	if got := worktreeEntryCount(t, fx.scratch); got != 1 {
		t.Errorf("worktree list must be exactly 1 entry; got %d", got)
	}
	if leftovers := releaseVerifyLeftovers(t, fx); len(leftovers) != 0 {
		t.Errorf("no verification worktree directory may exist; got %v", leftovers)
	}

	// --- source assertions: zero hook identifiers, zero dispatch forms ----
	src := string(srcBytes)
	if strings.Contains(src, "RELEASE_TAG_TEST_PRETAG_HOOK") {
		t.Fatalf("production source must carry ZERO occurrences of RELEASE_TAG_TEST_PRETAG_HOOK")
	}
	// Renamed/indirect replacements are pinned where they are mechanically
	// distinguishable from legitimate use: eval/source/dot-source as a
	// command, and `bash -c` dispatch. Variable-command dispatch is NOT
	// blanket-banned here: the wrapper legitimately invokes its fixed
	// ceremony binary via "$HARNESS_BIN" (a wrapper-resolved path, never
	// env-supplied) — reviewed manually; no env-supplied command dispatch
	// remains.
	for _, pat := range []struct {
		name string
		re   *regexp.Regexp
	}{
		{"eval command", regexp.MustCompile(`(?m)^[\t ]*eval[\t ]`)},
		{"source command", regexp.MustCompile(`(?m)^[\t ]*source[\t ]`)},
		{"dot-source command", regexp.MustCompile(`(?m)^[\t ]*\.[\t ]+\S`)},
		{"bash -c dispatch", regexp.MustCompile(`bash[\t ]+-c`)},
	} {
		if loc := pat.re.FindStringIndex(src); loc != nil {
			line := strings.Count(src[:loc[0]], "\n") + 1
			t.Fatalf("production source must not contain a %s (line %d) — an indirect replacement for the removed hook channel", pat.name, line)
		}
	}
}

// =============================================================================
// G0c RELEASE-CONTEXT ROUTING (fake ceremony binary)
// =============================================================================

// writeFakeHarnessBinary plants a bash shim at scratch/bin/vh-agent-harness
// that satisfies the G0c canary, answers the control doctor run HEALTHY,
// and serves $SCRATCH_FAKE_DOCTOR_JSON to the --target --json run. The
// --target arm mirrors the REAL binary's exit contract: doctor exits
// NON-ZERO whenever the report carries problems (healthy:false) while the
// full JSON envelope still lands on stdout — a fake that exits 0 alongside
// a FAIL payload would be unrepresentable of the real binary and would
// miss wrapper regressions that key off the exit code alone.
func writeFakeHarnessBinary(t *testing.T, scratch string) {
	t.Helper()
	shim := `#!/usr/bin/env bash
# Fake ceremony binary for the historical-M G0c routing tests.
if [ "$1" = "release" ] && [ "$2" = "inject-errata" ]; then
  exit 0
fi
if [ "$1" = "doctor" ] && [ "$2" = "--target" ]; then
  cat "$SCRATCH_FAKE_DOCTOR_JSON"
  # Real exit semantics: nonzero iff the envelope reports unhealthy.
  if grep -q '"healthy": *true' "$SCRATCH_FAKE_DOCTOR_JSON"; then
    exit 0
  fi
  exit 1
fi
if [ "$1" = "doctor" ]; then
  printf 'doctor:\n  lineage:\n    lineage      PASS  ok (fake control run)\n'
  printf 'result: HEALTHY\n'
  exit 0
fi
exit 1
`
	binDir := filepath.Join(scratch, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("mkdir bin: %v", err)
	}
	if err := os.WriteFile(filepath.Join(binDir, "vh-agent-harness"), []byte(shim), 0o755); err != nil {
		t.Fatalf("write fake harness binary: %v", err)
	}
}

// runHistoricalWithFakeDoctor wires the lineage + fake-binary fixture and
// the fake doctor JSON, returning the wrapper outcome.
func runHistoricalWithFakeDoctor(t *testing.T, doctorJSON string, extraArgs []string) (int, releaseTagHistoricalResult, *historicalFixture) {
	t.Helper()
	fx := setupHistoricalFixture(t, historicalFixtureOpts{Lineage: true})
	writeFakeHarnessBinary(t, fx.scratch)
	jsonFile := filepath.Join(t.TempDir(), "fake-doctor.json")
	if err := os.WriteFile(jsonFile, []byte(doctorJSON), 0o644); err != nil {
		t.Fatalf("write fake doctor json: %v", err)
	}
	msgFile := histMsgFile(t, "v0.2.0")
	exitCode, result, _, _ := runReleaseTagHistorical(t, fx.wrapper, fx.scratch, msgFile, "v0.2.0", extraArgs,
		map[string]string{"SCRATCH_FAKE_DOCTOR_JSON": jsonFile}, 120*time.Second)
	return exitCode, result, fx
}

// TestReleaseTag_Historical_TargetDoctorDefectAtM_Refuses — a target|both
// FAIL verdict in the release-context doctor run against X refuses: a
// defect at M refuses the tag even when repaired at H.
func TestReleaseTag_Historical_TargetDoctorDefectAtM_Refuses(t *testing.T) {
	doctorJSON := `{"schema_version":1,"checks":[` +
		`{"name":"lineage","tier":"PASS","class":"target","detail":"ok"},` +
		`{"name":"managed-drift","tier":"FAIL","class":"both","detail":"drift at M"}],` +
		`"unclassified":[],"coverage_complete":true,"problems":1,"warnings":0,"healthy":false}`
	exitCode, result, fx := runHistoricalWithFakeDoctor(t, doctorJSON, nil)
	if exitCode == 0 {
		t.Fatalf("target-class doctor FAIL at M must REFUSE; got exit 0")
	}
	if result.OK {
		t.Errorf("expected ok=false")
	}
	if result.Error == nil ||
		!strings.Contains(*result.Error, "TARGET failure at M") ||
		!strings.Contains(*result.Error, "managed-drift") ||
		!strings.Contains(*result.Error, "repaired at HEAD") {
		t.Errorf("error must name the target failure and the defect-at-M semantics; got %s", *result.Error)
	}
	if tagExists(t, fx.scratch, "v0.2.0") {
		t.Errorf("tag must NOT exist")
	}
	if got := worktreeEntryCount(t, fx.scratch); got != 1 {
		t.Errorf("worktree cleaned after refusal; got %d entries", got)
	}
}

// TestReleaseTag_Historical_TargetDoctorUnclassified_Refuses — an
// unregistered doctor check fails release-context coverage: an unknown
// check can never silently escape routing.
func TestReleaseTag_Historical_TargetDoctorUnclassified_Refuses(t *testing.T) {
	doctorJSON := `{"schema_version":1,"checks":[` +
		`{"name":"brand-new-check","tier":"PASS","class":"","detail":"?"},` +
		`{"name":"lineage","tier":"PASS","class":"target","detail":"ok"}],` +
		`"unclassified":["brand-new-check"],"coverage_complete":false,"problems":1,"warnings":0,"healthy":false}`
	exitCode, result, fx := runHistoricalWithFakeDoctor(t, doctorJSON, nil)
	if exitCode == 0 {
		t.Fatalf("unclassified doctor check must REFUSE (coverage incomplete); got exit 0")
	}
	if result.OK {
		t.Errorf("expected ok=false")
	}
	if result.Error == nil ||
		!strings.Contains(*result.Error, "coverage INCOMPLETE") ||
		!strings.Contains(*result.Error, "brand-new-check") {
		t.Errorf("error must name the unclassified check; got %s", *result.Error)
	}
	if tagExists(t, fx.scratch, "v0.2.0") {
		t.Errorf("tag must NOT exist")
	}
}

// TestReleaseTag_Historical_TargetDoctorMalformedEnvelope_Refuses — the
// round-2 vacuous-pass advisory plus the round-5 per-entry hardening
// (rejection #4 blockers C-F1/C-F2). The node class-router used to accept
// ANY parseable JSON: a `{}` envelope sailed past the `coverage_complete === false`
// probe (undefined is not false) and a missing/empty checks array produced
// zero routed failures — a vacuous ok:true, tag cut. The router now requires
// schema_version===1, BOOLEAN coverage_complete, and a NON-EMPTY checks
// array; anything else is treated as unparseable and refuses. RED receipt
// (round-2): on the pre-fix tree every shape below ACCEPTED and cut the tag.
// Round-5: even a top-level-VALID envelope with degenerate ENTRIES passed
// vacuously — checks:[null] and checks:[{}] name no check at all, and an
// entry whose tier sits outside the closed PASS/WARN/FAIL/SKIP/INFO
// vocabulary or whose class sits outside target|control|both can never carry
// a routable verdict, so out(true,"") fired with NO meaningful target
// verdict and the tag mutated. Every checks[] entry is now validated
// (non-null object, non-empty string name, closed-vocabulary tier,
// vocabulary class) after the coverage probe — the real emitter writes
// class:"" for unclassified checks with coverage_complete:false, and that
// envelope keeps its coverage-INCOMPLETE refusal. RED receipt (round-5):
// against the pre-fix wrapper (the round-5 slice-minus-this-fix baseline,
// bash -n clean) all four per-entry shapes ACCEPTED vacuously (exit 0, tag
// cut); post-fix every shape below refuses with the malformed-envelope
// diagnostic.
func TestReleaseTag_Historical_TargetDoctorMalformedEnvelope_Refuses(t *testing.T) {
	shapes := []string{
		// (a) no fields at all — missing schema_version, coverage_complete,
		// and checks in one payload.
		`{}`,
		// (b) right types but an EMPTY checks array — vacuous coverage.
		`{"schema_version":1,"checks":[],"unclassified":[],"coverage_complete":true,"problems":0,"warnings":0,"healthy":true}`,
		// (c) NON-BOOLEAN coverage_complete — the old `=== false` probe let
		// any non-false value through.
		`{"schema_version":1,"checks":[{"name":"lineage","tier":"PASS","class":"target","detail":"ok"}],"unclassified":[],"coverage_complete":"yes","problems":0,"warnings":0,"healthy":true}`,
		// (d) per-entry NULL — checks:[null] (round-5 C-F1). The top-level
		// shape is valid, but a null entry carries no verdict at all; the
		// pre-fix router vacuously passed out(true,"").
		`{"schema_version":1,"checks":[null],"unclassified":[],"coverage_complete":true,"problems":0,"warnings":0,"healthy":true}`,
		// (e) per-entry EMPTY OBJECT — checks:[{}] (round-5 C-F1): no
		// name/tier/class; same vacuous pass pre-fix.
		`{"schema_version":1,"checks":[{}],"unclassified":[],"coverage_complete":true,"problems":0,"warnings":0,"healthy":true}`,
		// (f) per-entry UNKNOWN TIER (round-5 C-F2) — a tier outside the
		// closed PASS/WARN/FAIL/SKIP/INFO vocabulary is not a verdict the
		// doctor emitter can produce.
		`{"schema_version":1,"checks":[{"name":"lineage","tier":"FATAL","class":"target","detail":"?"}],"unclassified":[],"coverage_complete":true,"problems":0,"warnings":0,"healthy":true}`,
		// (g) per-entry UNKNOWN CLASS (round-5 C-F2) — outside the routing
		// vocabulary target|control|both the entry can never be routed.
		`{"schema_version":1,"checks":[{"name":"lineage","tier":"PASS","class":"global","detail":"ok"}],"unclassified":[],"coverage_complete":true,"problems":0,"warnings":0,"healthy":true}`,
	}
	for i, doctorJSON := range shapes {
		exitCode, result, fx := runHistoricalWithFakeDoctor(t, doctorJSON, nil)
		if exitCode == 0 {
			t.Fatalf("shape %d: malformed doctor envelope must REFUSE; got exit 0", i)
		}
		if result.OK {
			t.Errorf("shape %d: expected ok=false", i)
		}
		if result.Error == nil || !strings.Contains(*result.Error, "unparseable") {
			t.Errorf("shape %d: error must name the unparseable/malformed envelope; got %s", i, *result.Error)
		}
		if tagExists(t, fx.scratch, "v0.2.0") {
			t.Errorf("shape %d: tag must NOT exist", i)
		}
	}
}

// control-class FAIL in the target run is IGNORED (the control run on the
// original checkout owns that verdict; X structurally lacks .local/ and
// repo-global state), and WARN verdicts never escalate — the run accepts.
func TestReleaseTag_Historical_TargetDoctorControlFailIgnored_Accepts(t *testing.T) {
	doctorJSON := `{"schema_version":1,"checks":[` +
		`{"name":"defer-liveness","tier":"FAIL","class":"control","detail":"adopted transport dir lost (structural in X)"},` +
		`{"name":"skills","tier":"WARN","class":"target","detail":"stale skill at M"}],` +
		`"unclassified":[],"coverage_complete":true,"problems":1,"warnings":1,"healthy":false}`
	exitCode, result, fx := runHistoricalWithFakeDoctor(t, doctorJSON, nil)
	if exitCode != 0 || !result.OK {
		t.Fatalf("control FAIL + target WARN must ACCEPT; exit=%d ok=%v error=%v", exitCode, result.OK, result.Error)
	}
	if !tagExists(t, fx.scratch, "v0.2.0") {
		t.Errorf("tag must exist")
	}
	if result.Commit == nil || *result.Commit != fx.msha {
		t.Errorf("commit must be M %s; got %v", fx.msha, result.Commit)
	}
	requireCleanHistoricalRepo(t, fx)
}

// TestReleaseTag_Historical_G0cRealBinary_ControlUnhealthyRefuses — with a
// REAL ceremony binary, the control doctor run on the ORIGINAL checkout
// must be HEALTHY; the scratch fixture (lineage.yml, no rendered corpus)
// is deliberately UNHEALTHY, so the historical flow refuses there — with
// the worktree cleaned up by the EXIT trap.
func TestReleaseTag_Historical_G0cRealBinary_ControlUnhealthyRefuses(t *testing.T) {
	fx := setupHistoricalFixture(t, historicalFixtureOpts{Lineage: true})
	copyHarnessBinaryToCeremony(t, fx.scratch)
	msgFile := histMsgFile(t, "v0.2.0")

	exitCode, result, _, _ := runReleaseTagHistorical(t, fx.wrapper, fx.scratch, msgFile, "v0.2.0", nil, nil, 240*time.Second)
	if exitCode == 0 {
		t.Fatalf("real-binary control doctor UNHEALTHY must REFUSE; got exit 0")
	}
	if result.OK {
		t.Errorf("expected ok=false")
	}
	if result.Error == nil || !strings.Contains(*result.Error, "G0c doctor not HEALTHY") {
		t.Errorf("error must name the control-run refusal; got %s", *result.Error)
	}
	if result.Historical == nil || !*result.Historical {
		t.Errorf("historical must be true; got %v", result.Historical)
	}
	if tagExists(t, fx.scratch, "v0.2.0") {
		t.Errorf("tag must NOT exist")
	}
	if got := worktreeEntryCount(t, fx.scratch); got != 1 {
		t.Errorf("worktree cleaned after refusal; got %d entries", got)
	}
}

// setupHistoricalSeamRenderFixture builds the historical fixture whose tree
// carries a FULL seam render (seamInstallInto) committed BELOW the ceremony —
// the only scratch shape the REAL ceremony binary's doctor reports HEALTHY
// against (a hand-written lineage.yml leaves doctor structurally unhealthy,
// which is the right red for ControlUnhealthyRefuses but wrong here).
// plantBelowCeremony, when non-nil, runs between the render commit and the
// ceremony and its effects are committed as one commit (the adoption-marker
// and lineage-poison plants of the real-binary arms below). The default
// chore commit H lands on top, exactly like setupHistoricalFixture.
func setupHistoricalSeamRenderFixture(t *testing.T, plantBelowCeremony func(t *testing.T, scratch string)) *historicalFixture {
	t.Helper()
	spec := manifestSpecForReadiness()
	scratch, wrapper, _, _, _ := setupReleaseTagManifestRepo(t, spec)
	fx := &historicalFixture{scratch: scratch, wrapper: wrapper, spec: spec}
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", scratch}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	// tmp/ hosts the verification worktree and .local/ hosts live coordinator
	// transport; both must be ignored BEFORE anything lands under them.
	commitScratchFile(t, scratch, ".gitignore", "bin/\ntmp/\n.local/\n", "gitignore tmp/local scratch")
	// Full seam render, committed below the ceremony so BOTH the control run
	// (H) and the target run (X, a checkout of M) see a real harness install.
	seamInstallInto(t, scratch)
	git("add", "-A")
	git("commit", "-q", "-m", "seam render fixture for real-doctor historical arm")
	if plantBelowCeremony != nil {
		plantBelowCeremony(t, scratch)
		git("add", "-A")
		git("commit", "-q", "-m", "below-ceremony plant for real-doctor historical arm")
	}
	// Ceremony: note (N) -> readiness artifact (R) -> manifest (M).
	commitScratchFile(t, scratch, "templates/migrations/v0.2.0.md", "# v0.2.0\n\nTest migration note for the historical fixture.\n", "migration note (N)")
	fx.nsha = gitRevParseVerify(t, scratch, "HEAD")
	artifactBytes := buildReadinessArtifactBytes(t, readinessArtifactSpec{CommitSHA: fx.nsha})
	commitScratchFile(t, scratch, ".vh-agent-harness/release-readiness-pass.json", string(artifactBytes), "readiness artifact (R)")
	fx.rsha = gitRevParseVerify(t, scratch, "HEAD")
	restampManifest(t, scratch, spec)
	fx.msha = gitRevParseVerify(t, scratch, "HEAD")

	if err := os.MkdirAll(filepath.Join(scratch, "tmp"), 0o755); err != nil {
		t.Fatalf("mkdir scratch tmp: %v", err)
	}
	branchOut, err := exec.Command("git", "-C", scratch, "symbolic-ref", "--short", "HEAD").Output()
	if err != nil {
		t.Fatalf("symbolic-ref: %v", err)
	}
	fx.branch = strings.TrimSpace(string(branchOut))
	fx.commitChore(t, "", "", "")
	return fx
}

// TestReleaseTag_Historical_G0cRealBinary_ControlOnlyFailAtX_Tolerated is the
// deterministic repo case at the wrapper level with the REAL ceremony binary:
// the repo commits the coordinator-adoption marker, and the verification
// worktree X (a checkout of M) structurally lacks the gitignored
// .local/coordinator/tasks transport — so defer-liveness state-matrix row 4
// (tasks absent + valid marker) FAILs on EVERY target run with real exit
// semantics (nonzero exit + full JSON envelope on stdout). The wrapper must
// TOLERATE it: defer-liveness is CONTROL-class (its live resources are
// H-side), so the class router ignores the FAIL and the release proceeds.
func TestReleaseTag_Historical_G0cRealBinary_ControlOnlyFailAtX_Tolerated(t *testing.T) {
	fx := setupHistoricalSeamRenderFixture(t, func(t *testing.T, scratch string) {
		// Committed below the ceremony: M's tree carries the adoption marker.
		writeAdoptionMarker(t, scratch)
	})
	// Live at H only (gitignored): the adopted transport dir the control run
	// needs for defer-liveness row 3. X never sees it — that asymmetry IS the
	// deterministic repo case.
	if err := os.MkdirAll(filepath.Join(fx.scratch, ".local", "coordinator", "tasks"), 0o755); err != nil {
		t.Fatalf("mkdir live tasks dir: %v", err)
	}
	copyHarnessBinaryToCeremony(t, fx.scratch)

	// PREMISE PIN: a real detached checkout of M (exactly what X is) must make
	// the REAL binary report a control-class defer-liveness FAIL with a
	// NONZERO exit and a complete JSON envelope — the real exit semantics the
	// wrapper's target-run capture has to survive. Without this premise the
	// accept below could be passing against an incidental all-green X.
	bin := filepath.Join(fx.scratch, "bin", "vh-agent-harness")
	wt := filepath.Join(t.TempDir(), "premise-wt")
	if out, err := exec.Command("git", "-C", fx.scratch, "worktree", "add", "--detach", "--quiet", wt, fx.msha).CombinedOutput(); err != nil {
		t.Fatalf("worktree add premise checkout: %v\n%s", err, out)
	}
	// Safety net only: the premise worktree is removed BEFORE the wrapper run
	// (below) so it never shows up in the post-run worktree-count assertion.
	t.Cleanup(func() {
		_ = exec.Command("git", "-C", fx.scratch, "worktree", "remove", "--force", wt).Run()
		_ = exec.Command("git", "-C", fx.scratch, "worktree", "prune").Run()
	})
	cmd := exec.Command(bin, "doctor", "--target", wt, "--json")
	out, runErr := cmd.Output()
	// PIN the run error explicitly (round-2 advisory): errors.As on a nil
	// error returns false, so the old
	// `errors.As(runErr, &exitErr) && exitErr.ExitCode() == 0` form could
	// NEVER detect a clean exit-0 doctor — the nonzero-exit premise was
	// vacuous. Pin runErr != nil (doctor exited NONZERO) and that it is an
	// ExitError (a real exit, not a spawn failure).
	if runErr == nil {
		t.Fatalf("premise: real doctor against a checkout of M must exit NONZERO (row-4 defer-liveness FAIL); got a clean exit 0")
	}
	var exitErr *exec.ExitError
	if !errors.As(runErr, &exitErr) {
		t.Fatalf("premise: doctor spawn error (not an exit): %v", runErr)
	}
	var premise doctorJSONReport
	if err := json.Unmarshal(out, &premise); err != nil {
		t.Fatalf("premise: doctor --json envelope must parse (exit=%v): %v\n%s", runErr, err, out)
	}
	var sawRow4 bool
	for _, c := range premise.Checks {
		if c.Name == "defer-liveness" {
			sawRow4 = true
			if c.Tier != tierFail {
				t.Errorf("premise: defer-liveness at a checkout of M must be FAIL (row 4); got %s", c.Tier)
			}
			if c.Class != releaseClassControl {
				t.Errorf("premise: defer-liveness must carry the CONTROL release-context class; got %s", c.Class)
			}
		}
		if c.Class == releaseClassTarget || c.Class == releaseClassBoth {
			if c.Tier == tierFail {
				t.Errorf("premise: target/both check %s unexpectedly FAIL at a checkout of M — the tolerate arm needs control-only FAILs; got %s", c.Name, c.Detail)
			}
		}
	}
	if !sawRow4 {
		t.Fatalf("premise: defer-liveness entry missing from envelope; checks=%v", premise.Checks)
	}
	// Remove the premise worktree NOW: the post-run assertions below expect
	// the scratch repo back at exactly one registered worktree.
	if out, err := exec.Command("git", "-C", fx.scratch, "worktree", "remove", "--force", wt).CombinedOutput(); err != nil {
		t.Fatalf("remove premise worktree: %v\n%s", err, out)
	}
	_ = exec.Command("git", "-C", fx.scratch, "worktree", "prune").Run()

	msgFile := histMsgFile(t, "v0.2.0")
	exitCode, result, _, stderr := runReleaseTagHistorical(t, fx.wrapper, fx.scratch, msgFile, "v0.2.0", nil, nil, 300*time.Second)
	// PIN the clean run explicitly (round-2 advisory): the tolerated arm's
	// wrapper run must be a CLEAN exit-0 run (runErr === nil at the helper
	// level — the helper fatals on spawn errors), not merely the absence of
	// ok:false.
	if exitCode != 0 {
		t.Fatalf("tolerated run: wrapper must run CLEAN (exit 0); got exit %d\nstderr:\n%s", exitCode, stderr)
	}
	if !result.OK {
		t.Fatalf("tolerated run: wrapper must report ok=true; error=%v", result.Error)
	}
	if result.Commit == nil || *result.Commit != fx.msha {
		t.Errorf("commit must be M %s; got %v", fx.msha, result.Commit)
	}
	if !tagExists(t, fx.scratch, "v0.2.0") {
		t.Errorf("tag must exist after the tolerated run")
	}
	if peeled := gitRevParseVerify(t, fx.scratch, "refs/tags/v0.2.0^{commit}"); peeled != fx.msha {
		t.Errorf("tag must peel to M %s; got %s", fx.msha, peeled)
	}
	requireCleanHistoricalRepo(t, fx)
	if got := worktreeEntryCount(t, fx.scratch); got != 1 {
		t.Errorf("worktree list must be back to exactly 1 entry; got %d", got)
	}
}

// TestReleaseTag_Historical_G0cRealBinary_TargetFailAtM_Refuses — the
// complement with the REAL ceremony binary: a target-class check defect that
// exists at M and is repaired at H still refuses. The plant poisons a
// rendered SKILL.md below the ceremony (the "skills" check is TARGET-class:
// it reads tracked frontmatter under .opencode/skills/); a second eligible
// chore restores the valid bytes at H, so the CONTROL run at H is HEALTHY —
// only the target run at X (a checkout of M) sees the defect, and the
// wrapper must refuse naming the check.
//
// Round-2 note: the arm originally poisoned .vh-agent-harness/lineage.yml
// and repaired it at H. lineage.yml has since joined the protected-path scan
// (the round-2 activation-bypass fix), so that repair is now itself the
// refusal the scan exists to catch — the lineage defect-at-M shape is no
// longer expressible at this seam. The defect moved to a non-protected
// target-class input (.opencode/skills/*/SKILL.md); the arm's meaning is
// unchanged: target-class defect at M, repaired at H, still refuses.
func TestReleaseTag_Historical_G0cRealBinary_TargetFailAtM_Refuses(t *testing.T) {
	skillRel := ".opencode/skills/backlog/SKILL.md"
	var validSkill []byte
	fx := setupHistoricalSeamRenderFixture(t, func(t *testing.T, scratch string) {
		// Committed below the ceremony: M's tree carries the coordinator
		// adoption marker (the control run's defer-liveness row 3 needs the
		// live tasks dir at H) AND the poisoned skill frontmatter.
		writeAdoptionMarker(t, scratch)
		orig, err := os.ReadFile(filepath.Join(scratch, filepath.FromSlash(skillRel)))
		if err != nil {
			t.Fatalf("read rendered skill: %v", err)
		}
		validSkill = orig
		// No frontmatter block at all — schema.ValidateSkillFrontmatter
		// rejects it, so doctor's skills check FAILs on this tree.
		poisoned := "poisoned skill body with no frontmatter block — invalid SKILL.md for the target-fail arm\n"
		if err := os.WriteFile(filepath.Join(scratch, filepath.FromSlash(skillRel)), []byte(poisoned), 0o644); err != nil {
			t.Fatalf("poison skill: %v", err)
		}
	})
	// Live at H only (gitignored): the adopted transport dir the control run
	// needs for defer-liveness row 3. X never sees it — control-class.
	if err := os.MkdirAll(filepath.Join(fx.scratch, ".local", "coordinator", "tasks"), 0o755); err != nil {
		t.Fatalf("mkdir live tasks dir: %v", err)
	}
	// A SECOND eligible chore above the default one repairs the defect at H
	// (a rendered SKILL.md is NOT a protected path — the eligibility scan
	// guards the ceremony surface plus the lineage record — and
	// byte-identical restoration keeps the control run at H HEALTHY). Only
	// the target run at X (checkout of M) sees the poison: exactly the
	// defect-at-M/repaired-at-H shape that must refuse.
	commitScratchFile(t, fx.scratch, skillRel, string(validSkill), "chore: restore the poisoned skill after M")
	fx.hsha = gitRevParseVerify(t, fx.scratch, "HEAD")
	copyHarnessBinaryToCeremony(t, fx.scratch)

	msgFile := histMsgFile(t, "v0.2.0")
	exitCode, result, _, _ := runReleaseTagHistorical(t, fx.wrapper, fx.scratch, msgFile, "v0.2.0", nil, nil, 300*time.Second)
	if exitCode == 0 {
		t.Fatalf("target-class FAIL at M must REFUSE even when repaired at H; got exit 0")
	}
	if result.OK {
		t.Errorf("expected ok=false")
	}
	if result.Error == nil ||
		!strings.Contains(*result.Error, "TARGET failure at M") ||
		!strings.Contains(*result.Error, "skills") {
		t.Errorf("error must be the routed diagnostic naming the target-class check; got %s", *result.Error)
	}
	if tagExists(t, fx.scratch, "v0.2.0") {
		t.Errorf("tag must NOT exist")
	}
	requireCleanHistoricalRepo(t, fx)
	if got := worktreeEntryCount(t, fx.scratch); got != 1 {
		t.Errorf("worktree cleaned after refusal; got %d entries", got)
	}
}

// Round-4 C-F1 fixture: the defer-liveness anchor-mismatch escape.
//
// deferLivenessEscapeNoteRel is a NON-PROTECTED path (docs/ — outside the
// ceremony surface the eligibility scan protects) that is ABSENT at the
// prior tag v0.1.0, added below the ceremony, and deleted again by an
// eligible post-M chore.
const (
	deferLivenessEscapeNoteRel = "docs/scratch-note.md"
	deferLivenessEscapeCardID  = "defer-hist-anchor-escape"
)

// gitDiffNameOnly returns the path set of `git diff --name-only a b` in the
// scratch repo (fixture premise pins) — rename detection at git defaults
// (ON for the porcelain diff), so a pure rename appears as ONLY its
// destination.
func gitDiffNameOnly(t *testing.T, scratch, a, b string) map[string]bool {
	t.Helper()
	return gitDiffNameOnlyEx(t, scratch, nil, a, b)
}

// gitDiffNameOnlyEx is gitDiffNameOnly with extra diff args. The F7 premise
// pin passes --no-renames: rename detection disabled, so a rename appears as
// BOTH its source (deleted) and destination (added) — the shape the
// protected-path scan relies on post-fix.
func gitDiffNameOnlyEx(t *testing.T, scratch string, extra []string, a, b string) map[string]bool {
	t.Helper()
	args := append([]string{"-C", scratch, "diff", "--name-only"}, extra...)
	args = append(args, a, b)
	out, err := exec.Command("git", args...).Output()
	if err != nil {
		t.Fatalf("git diff --name-only %v %s %s: %v", extra, a, b, err)
	}
	set := map[string]bool{}
	for _, line := range strings.Split(string(out), "\n") {
		if p := strings.TrimSpace(line); p != "" {
			set[p] = true
		}
	}
	return set
}

// setupDeferLivenessAnchorEscapeFixture builds the round-4 C-F1 escape shape
// on the real-binary seam render (setupHistoricalSeamRenderFixture):
//
//   - a NON-PROTECTED path (docs/scratch-note.md — absent at the prior tag
//     v0.1.0) is committed BELOW the ceremony, so the M release arc
//     (v0.1.0..M) touches it;
//   - a card in H's LIVE registry (gitignored, never committed — the only
//     live card state at tag time) carries
//     trigger:path_touched(docs/scratch-note.md); cardStatus parameterizes
//     its disposition ("draft" = the OPEN firing arm; a closed status is
//     the disposed negative control). The card body deliberately carries no
//     vX.Y.Z version references so doctor's released-claim contradiction
//     scan stays out of scope — recurrence is the only live axis.
//   - an eligible post-M chore deletes the path again — restoring the
//     prior-tag state (absent) — so the control run's one-ended diff at H
//     (v0.1.0..H) has NO entry for the path: exactly the restored-path
//     blindness of the C-F1 anchor mismatch.
//
// It returns the fixture with premise pins asserted (M arc touches the
// path; H diff does not) so the arms below can trust the escape shape.
func setupDeferLivenessAnchorEscapeFixture(t *testing.T, cardStatus string) *historicalFixture {
	t.Helper()
	fx := setupHistoricalSeamRenderFixture(t, func(t *testing.T, scratch string) {
		writeAdoptionMarker(t, scratch)
		if err := os.MkdirAll(filepath.Join(scratch, filepath.FromSlash(filepath.Dir(deferLivenessEscapeNoteRel))), 0o755); err != nil {
			t.Fatalf("mkdir docs: %v", err)
		}
		if err := os.WriteFile(filepath.Join(scratch, filepath.FromSlash(deferLivenessEscapeNoteRel)),
			[]byte("scratch content touched in the M release arc\n"), 0o644); err != nil {
			t.Fatalf("write scratch note: %v", err)
		}
	})
	// Live at H only (gitignored): the adopted transport dir plus the card
	// whose trigger fires on the M-arc path. X never sees either.
	tasksDir := filepath.Join(fx.scratch, ".local", "coordinator", "tasks")
	if err := os.MkdirAll(tasksDir, 0o755); err != nil {
		t.Fatalf("mkdir live tasks dir: %v", err)
	}
	card := map[string]interface{}{
		"task_id": deferLivenessEscapeCardID,
		"status":  cardStatus,
		"title":   "historical-M anchor-escape regression card",
		"owner_notes": []string{
			"source:review-defer",
			"trigger:path_touched(" + deferLivenessEscapeNoteRel + ")",
			"studied:2026-11-17",
		},
	}
	cardBytes, err := json.Marshal(card)
	if err != nil {
		t.Fatalf("marshal card: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tasksDir, deferLivenessEscapeCardID+".json"), cardBytes, 0o644); err != nil {
		t.Fatalf("write live card: %v", err)
	}
	// Eligible post-M chore ABOVE the default one: restore the path to its
	// prior-tag state. The path is ABSENT at v0.1.0, so the restoration is a
	// deletion; docs/scratch-note.md is not a protected path, so the chore
	// stays eligible (only the protected-path scan would refuse it).
	removeScratchFile(t, fx.scratch, deferLivenessEscapeNoteRel,
		"chore: restore scratch note to prior-tag state (absent at v0.1.0)")
	fx.hsha = gitRevParseVerify(t, fx.scratch, "HEAD")
	copyHarnessBinaryToCeremony(t, fx.scratch)

	// PREMISE PIN: the M arc touches the path and the H diff does not — the
	// exact two-sided blindness the supplement exists to close. Without this
	// pin the refusal below could rest on a fixture that never touched (or
	// never restored) the path.
	mArc := gitDiffNameOnly(t, fx.scratch, "v0.1.0", fx.msha)
	if !mArc[deferLivenessEscapeNoteRel] {
		t.Fatalf("premise: M arc (v0.1.0..%s) must touch %s; got %v", fx.msha, deferLivenessEscapeNoteRel, mArc)
	}
	hDiff := gitDiffNameOnly(t, fx.scratch, "v0.1.0", fx.hsha)
	if hDiff[deferLivenessEscapeNoteRel] {
		t.Fatalf("premise: H diff (v0.1.0..%s) must NOT touch %s (restored by the chore); got %v", fx.hsha, deferLivenessEscapeNoteRel, hDiff)
	}
	return fx
}

// TestReleaseTag_Historical_G0cRealBinary_DeferLivenessAnchorEscape_Refuses —
// round-4 C-F1 regression (real git history, real ceremony binary, real live
// registry): an OPEN undisposed card whose path_touched fires inside the M
// release arc while an eligible post-M chore restores the path to prior-tag
// bytes is invisible to BOTH G0c sides — the control run at H diffs
// PRIOR_TAG..H (no entry for the restored path) and the target run at M
// routes defer-liveness as control-class. Pre-fix, that escape cut the tag
// (the RED receipt replays it against the e865615 wrapper); the historical-M
// defer-liveness anchor supplement must REFUSE it, naming the firing card.
func TestReleaseTag_Historical_G0cRealBinary_DeferLivenessAnchorEscape_Refuses(t *testing.T) {
	fx := setupDeferLivenessAnchorEscapeFixture(t, "draft")
	msgFile := histMsgFile(t, "v0.2.0")
	exitCode, result, _, _ := runReleaseTagHistorical(t, fx.wrapper, fx.scratch, msgFile, "v0.2.0", nil, nil, 300*time.Second)
	if exitCode == 0 {
		t.Fatalf("OPEN firing card in the M arc must REFUSE despite the restored path; got exit 0 (the C-F1 escape)")
	}
	if result.OK {
		t.Errorf("expected ok=false")
	}
	if result.Error == nil ||
		!strings.Contains(*result.Error, "historical-M defer-liveness") ||
		!strings.Contains(*result.Error, deferLivenessEscapeCardID) ||
		!strings.Contains(*result.Error, deferLivenessEscapeNoteRel) {
		got := "(nil)"
		if result.Error != nil {
			got = *result.Error
		}
		t.Errorf("error must be the anchor-supplement refusal naming the firing card and its path; got %s", got)
	}
	if tagExists(t, fx.scratch, "v0.2.0") {
		t.Errorf("tag must NOT exist after the refusal")
	}
	requireCleanHistoricalRepo(t, fx)
	if got := worktreeEntryCount(t, fx.scratch); got != 1 {
		t.Errorf("worktree cleaned after refusal; got %d entries", got)
	}
}

// TestReleaseTag_Historical_G0cRealBinary_DeferLivenessAnchorEscape_DisposedCard_StillAccepts —
// the negative control over the identical escape shape: with the card
// DISPOSED (status completed — a closed disposition), the ceremony still
// accepts. Disposition satisfaction — not the mere presence of a card or an
// M-arc touch — is what the supplement demands.
func TestReleaseTag_Historical_G0cRealBinary_DeferLivenessAnchorEscape_DisposedCard_StillAccepts(t *testing.T) {
	fx := setupDeferLivenessAnchorEscapeFixture(t, "completed")
	msgFile := histMsgFile(t, "v0.2.0")
	exitCode, result, _, stderr := runReleaseTagHistorical(t, fx.wrapper, fx.scratch, msgFile, "v0.2.0", nil, nil, 300*time.Second)
	if exitCode != 0 {
		t.Fatalf("disposed card must still ACCEPT; got exit %d\nstderr:\n%s", exitCode, stderr)
	}
	if !result.OK {
		t.Fatalf("expected ok=true; error=%v", result.Error)
	}
	if result.Commit == nil || *result.Commit != fx.msha {
		t.Errorf("commit must be M %s; got %v", fx.msha, result.Commit)
	}
	if !tagExists(t, fx.scratch, "v0.2.0") {
		t.Errorf("tag must exist after the accepted run")
	}
	if peeled := gitRevParseVerify(t, fx.scratch, "refs/tags/v0.2.0^{commit}"); peeled != fx.msha {
		t.Errorf("tag must peel to M %s; got %s", fx.msha, peeled)
	}
	requireCleanHistoricalRepo(t, fx)
	if got := worktreeEntryCount(t, fx.scratch); got != 1 {
		t.Errorf("worktree list must be back to exactly 1 entry; got %d", got)
	}
}

// Round-4/7 companion (defer-release-tag-mdl-greenfield-skip-pin): the
// greenfield fail-safe of the historical-M defer-liveness anchor supplement.
//
// greenfieldManifestSpecForHistorical is the root-base (first-release) twin
// of manifestSpecForReadiness: kind=root attests a greenfield release whose
// whole history is the release arc — no prior tag exists to derive (the
// same first-release shape as TestCheckDefer_Manifest_RootBaseEvaluates
// WholeHistory / ..._RootBaseIgnoresHeadFallback).
func greenfieldManifestSpecForHistorical() manifestSpec {
	return manifestSpec{
		ReleaseBaseKind:     "root",
		ReconciliationScope: "whole repository history through evaluated_commit",
		Records:             []manifestRecordSpec{seededNoDiscloseInvalid("defer-seed")},
	}
}

// deferLivenessGreenfieldCardID is the live OPEN firing card for the
// greenfield arm — distinct from deferLivenessEscapeCardID so the two
// fixtures' cards stay distinguishable in any surfaced diagnostic.
const deferLivenessGreenfieldCardID = "defer-hist-greenfield-skip"

// setupDeferLivenessGreenfieldFixture builds the greenfield (no-prior-tag)
// twin of the round-4 anchor-escape shape: the SAME full seam render,
// coordinator adoption marker, below-ceremony touched path, live OPEN
// firing card (trigger:path_touched on that path), and post-M restore chore
// — on a root-base manifest with NO tag in the repo at all. The wrapper's
// PRIOR_TAG derivation (`git describe --tags --abbrev=0 <M>`) is therefore
// EMPTY, and the anchor supplement must take its GREENFIELD SKIP branch
// (release-tag.sh: `if [ "$HISTORICAL" = "1" ] && [ -n "$PRIOR_TAG" ]`)
// rather than refusing fail-closed on the unresolvable prior tag — the
// supplement mirrors doctor's own fail-safe-dormant recurrence predicate on
// the same boundary (evaluateDeferRecurrence with an empty diff-since
// self-derivation failure yields no blockers).
func setupDeferLivenessGreenfieldFixture(t *testing.T) *historicalFixture {
	t.Helper()
	spec := greenfieldManifestSpecForHistorical()
	scratch, wrapper, _, _, _ := setupReleaseTagManifestRepo(t, spec)
	// setupReleaseTagManifestRepo tags v0.1.0 mid-build; greenfield means
	// ZERO tags, so delete it (the recorded first-release simulation move).
	if out, err := exec.Command("git", "-C", scratch, "tag", "-d", "v0.1.0").CombinedOutput(); err != nil {
		t.Fatalf("git tag -d v0.1.0: %v\n%s", err, out)
	}
	fx := &historicalFixture{scratch: scratch, wrapper: wrapper, spec: spec}
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", scratch}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	// tmp/ hosts the verification worktree and .local/ hosts live coordinator
	// transport; both must be ignored BEFORE anything lands under them.
	commitScratchFile(t, scratch, ".gitignore", "bin/\ntmp/\n.local/\n", "gitignore tmp/local scratch")
	// Full seam render + the escape shape's below-ceremony plant (adoption
	// marker + the note the card's trigger points at), committed below the
	// ceremony so M's tree carries both.
	seamInstallInto(t, scratch)
	writeAdoptionMarker(t, scratch)
	if err := os.MkdirAll(filepath.Join(scratch, filepath.FromSlash(filepath.Dir(deferLivenessEscapeNoteRel))), 0o755); err != nil {
		t.Fatalf("mkdir docs: %v", err)
	}
	if err := os.WriteFile(filepath.Join(scratch, filepath.FromSlash(deferLivenessEscapeNoteRel)),
		[]byte("scratch content touched before the greenfield ceremony\n"), 0o644); err != nil {
		t.Fatalf("write scratch note: %v", err)
	}
	git("add", "-A")
	git("commit", "-q", "-m", "seam render + greenfield anchor-skip plant")

	// Ceremony: note (N) -> readiness artifact (R) -> manifest (M), with the
	// greenfield root-base manifest spec.
	commitScratchFile(t, scratch, "templates/migrations/v0.2.0.md", "# v0.2.0\n\nTest migration note for the historical fixture.\n", "migration note (N)")
	fx.nsha = gitRevParseVerify(t, scratch, "HEAD")
	artifactBytes := buildReadinessArtifactBytes(t, readinessArtifactSpec{CommitSHA: fx.nsha})
	commitScratchFile(t, scratch, ".vh-agent-harness/release-readiness-pass.json", string(artifactBytes), "readiness artifact (R)")
	fx.rsha = gitRevParseVerify(t, scratch, "HEAD")
	restampManifest(t, scratch, spec)
	fx.msha = gitRevParseVerify(t, scratch, "HEAD")

	if err := os.MkdirAll(filepath.Join(scratch, "tmp"), 0o755); err != nil {
		t.Fatalf("mkdir scratch tmp: %v", err)
	}
	branchOut, err := exec.Command("git", "-C", scratch, "symbolic-ref", "--short", "HEAD").Output()
	if err != nil {
		t.Fatalf("symbolic-ref: %v", err)
	}
	fx.branch = strings.TrimSpace(string(branchOut))

	// Eligible post-M chore: the escape shape's restore (delete the note
	// again). In greenfield there is no prior-tag baseline to restore to,
	// but keeping the chore makes this fixture byte-parallel to the
	// anchor-escape one minus the tag — the differential IS the pin.
	removeScratchFile(t, scratch, deferLivenessEscapeNoteRel,
		"chore: remove the scratch note above M (greenfield twin of the restore chore)")
	fx.hsha = gitRevParseVerify(t, scratch, "HEAD")

	// Live at H only (gitignored): the adopted transport dir plus the OPEN
	// firing card — the exact card shape the supplement REFUSES when a prior
	// tag exists (round-4 C-F1 arm).
	tasksDir := filepath.Join(scratch, ".local", "coordinator", "tasks")
	if err := os.MkdirAll(tasksDir, 0o755); err != nil {
		t.Fatalf("mkdir live tasks dir: %v", err)
	}
	card := map[string]interface{}{
		"task_id": deferLivenessGreenfieldCardID,
		"status":  "draft",
		"title":   "historical-M greenfield anchor-skip regression card",
		"owner_notes": []string{
			"source:review-defer",
			"trigger:path_touched(" + deferLivenessEscapeNoteRel + ")",
			"studied:2026-11-18",
		},
	}
	cardBytes, err := json.Marshal(card)
	if err != nil {
		t.Fatalf("marshal card: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tasksDir, deferLivenessGreenfieldCardID+".json"), cardBytes, 0o644); err != nil {
		t.Fatalf("write live card: %v", err)
	}
	copyHarnessBinaryToCeremony(t, scratch)

	// PREMISE PINS — the load-bearing facts the skip rests on:
	// (1) ZERO tags exist, so the wrapper's PRIOR_TAG derivation at the
	//     derived M is empty (the greenfield boundary);
	// (2) the card's trigger path WAS committed in history (greenfield's
	//     release arc is the whole history, so the card is firing-shaped —
	//     had the supplement's PRIOR_TAG guard been absent, it would have
	//     refused fail-closed on the unresolvable prior tag).
	tagList, err := exec.Command("git", "-C", scratch, "tag").Output()
	if err != nil {
		t.Fatalf("git tag: %v", err)
	}
	if got := strings.TrimSpace(string(tagList)); got != "" {
		t.Fatalf("premise: greenfield fixture must carry ZERO tags (PRIOR_TAG derives empty); got %q", got)
	}
	touchLog, err := exec.Command("git", "-C", scratch, "log", "--format=%H", "--", deferLivenessEscapeNoteRel).Output()
	if err != nil {
		t.Fatalf("git log -- %s: %v", deferLivenessEscapeNoteRel, err)
	}
	if len(strings.Fields(string(touchLog))) == 0 {
		t.Fatalf("premise: the card's trigger path %s must have been committed in history", deferLivenessEscapeNoteRel)
	}
	return fx
}

// TestReleaseTag_Historical_G0cRealBinary_DeferLivenessAnchorGreenfield_NoPriorTag_SkipsAndAccepts —
// pin the supplement's intentional SKIP-when-PRIOR_TAG-empty branch
// (defer-release-tag-mdl-greenfield-skip-pin, round-4 A-F3): a greenfield
// historical-M run — real git history, real ceremony binary, real live
// registry, ZERO tags — carries the SAME OPEN firing card and touched path
// that the round-4 arm refuses when a prior tag exists, yet must ACCEPT:
// the supplement skips identically to doctor's fail-safe-dormant recurrence
// predicate (nothing to diff against) instead of inventing a stricter
// window or erroring on the empty PRIOR_TAG. The ACCEPT is the pin: had the
// `[ -n "$PRIOR_TAG" ]` guard been removed, the supplement's MDL_SINCE
// resolution would fail-closed ("could not resolve prior tag") and this
// exact fixture would refuse — no anchor-supplement refusal category may
// fire on a first release.
func TestReleaseTag_Historical_G0cRealBinary_DeferLivenessAnchorGreenfield_NoPriorTag_SkipsAndAccepts(t *testing.T) {
	fx := setupDeferLivenessGreenfieldFixture(t)
	msgFile := histMsgFile(t, "v0.2.0")
	exitCode, result, _, stderr := runReleaseTagHistorical(t, fx.wrapper, fx.scratch, msgFile, "v0.2.0", nil, nil, 300*time.Second)
	if exitCode != 0 {
		t.Fatalf("greenfield (no prior tag) historical-M must SKIP the anchor supplement and ACCEPT; got exit %d\nstderr:\n%s", exitCode, stderr)
	}
	if !result.OK {
		t.Fatalf("expected ok=true (the skip is not an error); error=%v", result.Error)
	}
	if result.Error != nil {
		t.Errorf("expected no error envelope on the accepted greenfield run; got %s", *result.Error)
	}
	if result.Commit == nil || *result.Commit != fx.msha {
		t.Errorf("commit must be the derived M %s; got %v", fx.msha, result.Commit)
	}
	if result.Historical == nil || !*result.Historical {
		t.Errorf("historical must be true; got %v", result.Historical)
	}
	// The first-ever tag: v0.2.0 peels to exactly M.
	if !tagExists(t, fx.scratch, "v0.2.0") {
		t.Fatalf("tag v0.2.0 must exist after the accepted greenfield run")
	}
	if peeled := gitRevParseVerify(t, fx.scratch, "refs/tags/v0.2.0^{commit}"); peeled != fx.msha {
		t.Errorf("tag must peel to M %s; got %s", fx.msha, peeled)
	}
	requireCleanHistoricalRepo(t, fx)
	if got := worktreeEntryCount(t, fx.scratch); got != 1 {
		t.Errorf("worktree list must be back to exactly 1 entry; got %d", got)
	}
	if leftovers := releaseVerifyLeftovers(t, fx); len(leftovers) != 0 {
		t.Errorf("verification worktree leaked: %v", leftovers)
	}
}

// realHistoryClone clones the module root (shared objects, no tags) and
// detaches HEAD at shortSHA. Skips when the recorded history is absent.
func realHistoryClone(t *testing.T, shortSHA, label string) (cloneDir, wrapperCopy string) {
	t.Helper()
	for _, bin := range []string{"bash", "git", "node"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("%s not on PATH: %v", bin, err)
		}
	}
	requireNodeMinMajor(t)
	root := findModuleRoot(t)
	probe := exec.Command("git", "-C", root, "rev-parse", "--verify", "--quiet", shortSHA+"^{commit}")
	if err := probe.Run(); err != nil {
		t.Skipf("recorded history fixture %s (%s) not resolvable in this checkout; skipping", label, shortSHA)
	}
	cloneDir = filepath.Join(t.TempDir(), "clone")
	clone := exec.Command("git", "clone", "-q", "--shared", "--no-tags", root, cloneDir)
	if out, err := clone.CombinedOutput(); err != nil {
		t.Fatalf("git clone --shared: %v\n%s", err, out)
	}
	if err := exec.Command("git", "-C", cloneDir, "checkout", "-q", "--detach", shortSHA).Run(); err != nil {
		t.Fatalf("git checkout --detach %s: %v", shortSHA, err)
	}
	// Run the CURRENT wrapper (working tree copy), not the checked-out one.
	wrapperBody, err := os.ReadFile(filepath.Join(root, "scripts", "release-tag.sh"))
	if err != nil {
		t.Fatalf("read current wrapper: %v", err)
	}
	wrapperCopy = filepath.Join(t.TempDir(), "release-tag.sh")
	if err := os.WriteFile(wrapperCopy, wrapperBody, 0o755); err != nil {
		t.Fatalf("write wrapper copy: %v", err)
	}
	return cloneDir, wrapperCopy
}

// TestReleaseTag_Historical_RealHistory_H7f94070_DerivesM1 — at the
// recorded H=7f94070a (chore above the completed v0.29.0 ceremony M1), the
// wrapper's derivation identifies M1=a9eb029f:
//
//	(a) --at <M2> refuses naming M1 as the eligible ceremony commit —
//	    M2 (dc6697c, the re-ceremony manifest) is NOT eligible from that
//	    H, proving the anchors select M1;
//	(b) --at <M1> passes the assertion (the refusal that follows comes
//	    from the ceremony surface checked out at that historical H — its
//	    evaluator predates --commit — never from the assertion).
func TestReleaseTag_Historical_RealHistory_H7f94070_DerivesM1(t *testing.T) {
	cloneDir, wrapperCopy := realHistoryClone(t, "7f94070", "H1 chore")
	m1 := gitRevParseVerify(t, cloneDir, "a9eb029f")
	m2 := gitRevParseVerify(t, cloneDir, "dc6697c")
	msgFile := histMsgFile(t, "v0.29.0")

	// (a) --at <M2> refuses; the derived eligible ceremony commit is M1.
	exitCode, result, _, _ := runReleaseTagHistorical(t, wrapperCopy, cloneDir, msgFile, "v0.29.0",
		[]string{"--at", m2}, nil, 60*time.Second)
	if exitCode == 0 {
		t.Fatalf("--at <M2> at H=7f94070 must REFUSE; got exit 0")
	}
	if result.OK {
		t.Errorf("expected ok=false")
	}
	if result.Error == nil ||
		!strings.Contains(*result.Error, "--at assertion failed") ||
		!strings.Contains(*result.Error, m1) {
		t.Errorf("error must name the derived eligible M1 (%s); got %v", m1, result.Error)
	}
	if result.Historical == nil || !*result.Historical {
		t.Errorf("historical must be true at H=7f94070; got %v", result.Historical)
	}

	// (b) --at <M1> passes the assertion — derivation accepts M1.
	exitCode2, result2, _, _ := runReleaseTagHistorical(t, wrapperCopy, cloneDir, msgFile, "v0.29.0",
		[]string{"--at", m1}, nil, 240*time.Second)
	if exitCode2 == 0 {
		t.Fatalf("historical-surface run must ultimately REFUSE (recorded ceremony surface); got exit 0")
	}
	if result2.OK {
		t.Errorf("expected ok=false")
	}
	if result2.Error != nil && strings.Contains(*result2.Error, "--at assertion failed") {
		t.Errorf("--at <M1> must PASS the assertion at H=7f94070 (derivation selects M1); got %v", result2.Error)
	}
}

// TestReleaseTag_Historical_RealHistory_AtM2_SupersededM1_Refuses — at the
// recorded H=M2 (dc6697c, the re-ceremony manifest commit, classic flow),
// --at <M1=a9eb029f> refuses naming M2: the current committed anchors
// select M2 and the superseded M1 can never be re-selected. The clone is
// tagless, so published-tag immutability cannot mask the eligibility
// refusal.
func TestReleaseTag_Historical_RealHistory_AtM2_SupersededM1_Refuses(t *testing.T) {
	cloneDir, wrapperCopy := realHistoryClone(t, "dc6697c", "M2 re-ceremony manifest")
	m1 := gitRevParseVerify(t, cloneDir, "a9eb029f")
	m2 := gitRevParseVerify(t, cloneDir, "dc6697c")
	msgFile := histMsgFile(t, "v0.29.0")

	if tagExists(t, cloneDir, "v0.29.0") {
		t.Fatalf("clone must be tagless so immutability cannot mask the superseded refusal")
	}

	exitCode, result, _, _ := runReleaseTagHistorical(t, wrapperCopy, cloneDir, msgFile, "v0.29.0",
		[]string{"--at", m1}, nil, 60*time.Second)
	if exitCode == 0 {
		t.Fatalf("--at <superseded M1> at H=M2 must REFUSE; got exit 0")
	}
	if result.OK {
		t.Errorf("expected ok=false")
	}
	if result.Error == nil ||
		!strings.Contains(*result.Error, "--at assertion failed") ||
		!strings.Contains(*result.Error, m2) ||
		!strings.Contains(*result.Error, m1) ||
		!strings.Contains(*result.Error, "superseded") {
		t.Errorf("error must name M2 as eligible, M1 as requested, and the superseded semantics; got %s", *result.Error)
	}
	if result.Historical == nil || *result.Historical {
		t.Errorf("historical must be FALSE in the classic flow at H=M2; got %v", result.Historical)
	}
	if tagExists(t, cloneDir, "v0.29.0") {
		t.Errorf("tag must NOT exist")
	}
}
