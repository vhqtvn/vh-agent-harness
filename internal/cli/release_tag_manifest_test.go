package cli

// Manifest-authority wrapper tests for scripts/release-tag.sh.
//
// Release mode is manifest-authority ONLY (the legacy .local/-scan release
// path has been RETIRED; manifest authority is the sole release-authority
// model). The wrapper passes --release-version $VERSION to the evaluator,
// performs the operator-side override ceremony against the actual manifest
// blob SHA (--override-release-version + --override-manifest-sha), and
// forwards --override-confirmed-version only when the ceremony succeeds.
// Disclosures and accepted overrides are embedded in the wrapper's JSON
// output and printed to stderr before any `git tag` mutation.
//
// This file pins the matrix cases 21–26 from the mission.

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// releaseTagManifestResult deserializes the manifest-authority wrapper's JSON
// output. It carries the base wrapper fields plus the optional disclosure and
// accepted-override arrays emitted by manifest-mode releases.
type releaseTagManifestResult struct {
	OK                bool                     `json:"ok"`
	Tag               *string                  `json:"tag"`
	Commit            *string                  `json:"commit"`
	Pushed            bool                     `json:"pushed"`
	Error             *string                  `json:"error"`
	Disclosures       []map[string]interface{} `json:"disclosures"`
	AcceptedOverrides []map[string]interface{} `json:"accepted_overrides"`
}

// setupReleaseTagManifestRepo mirrors setupReleaseTagRepo but additionally
// commits a manifest at .vh-agent-harness/release-defer-dispositions.json in
// the manifest-only immediate-child commit (HEAD^..HEAD diff is exactly the
// manifest path). The manifest's handshake SHAs are filled in from the
// pre-manifest HEAD. Returns the scratch dir, wrapper path, manifest path,
// manifest blob SHA, and the parent commit (P) that the manifest evaluates.
func setupReleaseTagManifestRepo(t *testing.T, spec manifestSpec) (scratch, wrapper, manifestPath, manifestSHA, parentCommit string) {
	t.Helper()
	for _, bin := range []string{"bash", "git", "node"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("%s not on PATH: %v", bin, err)
		}
	}
	root := findModuleRoot(t)
	wrapperBody, err := os.ReadFile(filepath.Join(root, "scripts", "release-tag.sh"))
	if err != nil {
		t.Fatalf("read wrapper: %v", err)
	}
	evalSrc := filepath.Join(root, "templates", "core", ".opencode", "scripts", "check-defer-triggers.js")
	evalBody, err := os.ReadFile(evalSrc)
	if err != nil {
		t.Fatalf("read evaluator template: %v", err)
	}
	renderedEval := strings.ReplaceAll(string(evalBody), "{{COORDINATOR_DIR}}", "coordinator")

	scratch = t.TempDir()
	wrapper = filepath.Join(scratch, "scripts", "release-tag.sh")
	if err := os.MkdirAll(filepath.Dir(wrapper), 0o755); err != nil {
		t.Fatalf("mkdir scripts: %v", err)
	}
	if err := os.WriteFile(wrapper, wrapperBody, 0o755); err != nil {
		t.Fatalf("write wrapper: %v", err)
	}
	evalDst := filepath.Join(scratch, ".opencode", "scripts", "check-defer-triggers.js")
	if err := os.MkdirAll(filepath.Dir(evalDst), 0o755); err != nil {
		t.Fatalf("mkdir .opencode/scripts: %v", err)
	}
	if err := os.WriteFile(evalDst, []byte(renderedEval), 0o644); err != nil {
		t.Fatalf("write evaluator: %v", err)
	}

	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", scratch}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git("init", "-q")
	git("config", "user.email", "t@t")
	git("config", "user.name", "t")
	git("config", "commit.gpgsign", "false")

	writeFile := func(rel, content string) {
		t.Helper()
		full := filepath.Join(scratch, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", rel, err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}
	writeFile("fileA.go", "package main\n")
	writeFile("fileB.go", "package main\n")
	writeFile("dir/fileC.go", "package dir\n")
	git("add", "-A")
	git("commit", "-q", "-m", "initial")
	git("tag", "v0.1.0")
	writeFile("fileA.go", "package main\n// changed in arc\n")
	writeFile("dir/fileC.go", "package dir\n// changed in arc\n")
	git("add", "-A")
	git("commit", "-q", "-m", "changes for release")

	// Commit ONLY the manifest in the immediate child commit.
	parentCommit = gitRevParseVerify(t, scratch, "HEAD")
	manifestBytes := buildManifestBytes(t, scratch, spec)
	manifestPath = commitReleaseManifest(t, scratch, manifestBytes, "")
	// Compute manifest blob SHA via git hash-object; on a clean post-commit
	// tree this equals the wrapper's git rev-parse HEAD:<path>.
	out, err := exec.Command("git", "-C", scratch, "hash-object", ".vh-agent-harness/release-defer-dispositions.json").Output()
	if err != nil {
		t.Fatalf("hash-object manifest: %v", err)
	}
	manifestSHA = strings.TrimSpace(string(out))
	return scratch, wrapper, manifestPath, manifestSHA, parentCommit
}

// runReleaseTagManifest invokes the wrapper in manifest-authority mode
// (always-on post-retirement; no env switch required). Optional extra args
// are appended after $1 (for --override-* flags).
//
// cwd is <scratch> so the wrapper resolves .opencode/scripts/check-defer-triggers.js
// and the manifest under .vh-agent-harness/release-defer-dispositions.json
// relative to the scratch repo root — the wrapper references them by relative
// path, so it MUST be launched from there.
func runReleaseTagManifest(t *testing.T, wrapper, msgFile, version string, extraArgs []string) (int, releaseTagManifestResult, string, string, string) {
	t.Helper()
	args := []string{wrapper, version}
	args = append(args, extraArgs...)
	cmd := exec.Command("bash", args...)
	cmd.Dir = filepath.Dir(filepath.Dir(wrapper)) // <scratch>
	cmd.Env = append(os.Environ(),
		"RELEASE_TAG_MESSAGE_FILE="+msgFile,
	)
	var outb, errb strings.Builder
	cmd.Stdout = &outb
	cmd.Stderr = &errb
	timer := time.AfterFunc(30*time.Second, func() {
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
	var result releaseTagManifestResult
	if stdout != "" {
		if err := json.Unmarshal([]byte(stdout), &result); err != nil {
			t.Fatalf("wrapper output must be valid JSON (exit=%d): %v\nstdout:\n%s\nstderr:\n%s",
				exitCode, err, stdout, stderr)
		}
	}
	return exitCode, result, stdout, stderr, ""
}

// =============================================================================
// HAPPY PATHS — manifest mode allows the tag
// =============================================================================

// TestReleaseTag_Manifest_NoDiscloseRecord_Allows — single no+disclose record
// is disclosed and the tag is created.
func TestReleaseTag_Manifest_NoDiscloseRecord_Allows(t *testing.T) {
	scratch, wrapper, _, _, _ := setupReleaseTagManifestRepo(t, manifestSpec{
		ReleaseBaseKind:     "tag",
		ReleaseBaseValue:    "v0.1.0",
		ReconciliationScope: "release arc from v0.1.0 through evaluated_commit",
		Records:             []manifestRecordSpec{seededNoDiscloseInvalid("defer-seed")},
	})
	msgFile := filepath.Join(scratch, "msg.txt")
	if err := os.WriteFile(msgFile, []byte("release v0.2.0\n\n-test\n"), 0o644); err != nil {
		t.Fatalf("write msg: %v", err)
	}
	exitCode, result, _, stderr, _ := runReleaseTagManifest(t, wrapper, msgFile, "v0.2.0", nil)
	if exitCode != 0 {
		t.Fatalf("manifest no-disclose must ALLOW (exit 0); got %d", exitCode)
	}
	if !result.OK {
		t.Errorf("expected ok=true; got false (error=%v)", result.Error)
	}
	if !tagExists(t, scratch, "v0.2.0") {
		t.Errorf("tag v0.2.0 must exist")
	}
	// Disclosures must be embedded AND printed to stderr before tag mutation.
	if len(result.Disclosures) != 1 {
		t.Errorf("disclosures must have 1 entry; got %v", result.Disclosures)
	}
	if !strings.Contains(stderr, "defer-seed") {
		t.Errorf("stderr must include disclosure print before tag; got:\n%s", stderr)
	}
}

// =============================================================================
// OVERRIDE CEREMONY — matrix cases 21–26
// =============================================================================

// overrideRequiredRecord returns a yes+override_required+valid record whose
// override is bound to releaseVersion.
func overrideRequiredRecord(id, releaseVersion string) manifestRecordSpec {
	return manifestRecordSpec{
		DeferID:          id,
		ReleaseRelevance: "yes",
		Disposition:      "override_required",
		MetadataState:    "valid",
		Summary:          "Release-relevant finding pending operator override.",
		Reason:           "Operator may accept residual risk for this release version.",
		SourceRef:        ".local/coordinator/tasks/" + id + ".json",
		StudiedAt:        "2026-07-15T00:00:00Z",
		ReviewedAt:       "2026-07-20T00:00:00Z",
		Override: &manifestOverrideSpec{
			ReleaseVersion: releaseVersion,
			ApprovedBy:     "operator-alice",
			ApprovedAt:     "2026-07-20T00:00:00Z",
			Reason:         "Acceptable residual risk; mitigation tracked separately.",
		},
	}
}

// TestReleaseTag_Manifest_OverrideRequiredNoConfirmationRefuses — matrix case
// 21. Override_required record committed with override.release_version=v0.2.0
// but operator runs the wrapper WITHOUT --override-* flags. The evaluator
// accepts the override (Layer A: well-formed committed object matching the
// release version — CI defense-in-depth contract), but the wrapper's post-
// evaluator ceremony gate refuses BEFORE the tag mutation because Layer B
// (operator live intent) was not supplied. This preserves wrapper enforcement
// of operator confirmation without weakening CI's verification role.
func TestReleaseTag_Manifest_OverrideRequiredNoConfirmationRefuses(t *testing.T) {
	scratch, wrapper, _, _, _ := setupReleaseTagManifestRepo(t, manifestSpec{
		ReleaseBaseKind:     "tag",
		ReleaseBaseValue:    "v0.1.0",
		ReconciliationScope: "release arc from v0.1.0 through evaluated_commit",
		Records:             []manifestRecordSpec{overrideRequiredRecord("defer-ov", "v0.2.0")},
	})
	msgFile := filepath.Join(scratch, "msg.txt")
	if err := os.WriteFile(msgFile, []byte("release v0.2.0\n\n-test\n"), 0o644); err != nil {
		t.Fatalf("write msg: %v", err)
	}
	exitCode, result, _, _, _ := runReleaseTagManifest(t, wrapper, msgFile, "v0.2.0", nil)
	if exitCode == 0 {
		t.Fatalf("override_required without ceremony must REFUSE; got exit 0")
	}
	if result.OK {
		t.Errorf("expected ok=false")
	}
	if tagExists(t, scratch, "v0.2.0") {
		t.Errorf("tag must NOT exist after refusal")
	}
	// The wrapper-side ceremony gate owns this refusal now. The error must
	// mention the ceremony requirement and carry the accepted override ID.
	if result.Error == nil || !strings.Contains(*result.Error, "ceremony") {
		t.Errorf("error must mention ceremony; got %v", result.Error)
	}
	if result.Error == nil || !strings.Contains(*result.Error, "defer-ov") {
		t.Errorf("error must carry the accepted override ID defer-ov; got %v", result.Error)
	}
}

// TestReleaseTag_Manifest_OverrideReleaseVersionMismatchRefuses — matrix case
// 22. Operator passes --override-release-version that does NOT match $VERSION.
// The wrapper refuses at the ceremony check, BEFORE invoking the evaluator
// (and BEFORE any git tag).
func TestReleaseTag_Manifest_OverrideReleaseVersionMismatchRefuses(t *testing.T) {
	scratch, wrapper, _, manifestSHA, _ := setupReleaseTagManifestRepo(t, manifestSpec{
		ReleaseBaseKind:     "tag",
		ReleaseBaseValue:    "v0.1.0",
		ReconciliationScope: "release arc from v0.1.0 through evaluated_commit",
		Records:             []manifestRecordSpec{overrideRequiredRecord("defer-ov", "v0.2.0")},
	})
	msgFile := filepath.Join(scratch, "msg.txt")
	if err := os.WriteFile(msgFile, []byte("release v0.2.0\n\n-test\n"), 0o644); err != nil {
		t.Fatalf("write msg: %v", err)
	}
	exitCode, result, _, _, _ := runReleaseTagManifest(t, wrapper, msgFile, "v0.2.0",
		[]string{"--override-release-version", "v0.99.0", "--override-manifest-sha", manifestSHA})
	if exitCode == 0 {
		t.Fatalf("override-release-version mismatch must REFUSE; got exit 0")
	}
	if result.OK {
		t.Errorf("expected ok=false")
	}
	if tagExists(t, scratch, "v0.2.0") {
		t.Errorf("tag must NOT exist after refusal")
	}
	if result.Error == nil || !strings.Contains(*result.Error, "override-release-version") {
		t.Errorf("error must mention override-release-version mismatch; got %v", result.Error)
	}
}

// TestReleaseTag_Manifest_OverrideManifestShaMismatchRefuses — matrix case 23.
// Operator passes --override-manifest-sha that does NOT match the actual blob
// SHA of the committed manifest. Refuses at the ceremony check.
func TestReleaseTag_Manifest_OverrideManifestShaMismatchRefuses(t *testing.T) {
	scratch, wrapper, _, _, _ := setupReleaseTagManifestRepo(t, manifestSpec{
		ReleaseBaseKind:     "tag",
		ReleaseBaseValue:    "v0.1.0",
		ReconciliationScope: "release arc from v0.1.0 through evaluated_commit",
		Records:             []manifestRecordSpec{overrideRequiredRecord("defer-ov", "v0.2.0")},
	})
	msgFile := filepath.Join(scratch, "msg.txt")
	if err := os.WriteFile(msgFile, []byte("release v0.2.0\n\n-test\n"), 0o644); err != nil {
		t.Fatalf("write msg: %v", err)
	}
	fakeSHA := strings.Repeat("a", 40)
	exitCode, result, _, _, _ := runReleaseTagManifest(t, wrapper, msgFile, "v0.2.0",
		[]string{"--override-release-version", "v0.2.0", "--override-manifest-sha", fakeSHA})
	if exitCode == 0 {
		t.Fatalf("override-manifest-sha mismatch must REFUSE; got exit 0")
	}
	if result.OK {
		t.Errorf("expected ok=false")
	}
	if tagExists(t, scratch, "v0.2.0") {
		t.Errorf("tag must NOT exist after refusal")
	}
	if result.Error == nil || !strings.Contains(*result.Error, "manifest-sha") {
		t.Errorf("error must mention manifest-sha mismatch; got %v", result.Error)
	}
}

// TestReleaseTag_Manifest_OverrideValidAllowsAndDiscloses — matrix case 24.
// Operator supplies consistent --override-release-version AND the actual
// manifest blob SHA. The wrapper forwards --override-confirmed-version, the
// evaluator accepts the override, the wrapper prints the accepted override to
// stderr and embeds it in the JSON, and the tag is created.
func TestReleaseTag_Manifest_OverrideValidAllowsAndDiscloses(t *testing.T) {
	scratch, wrapper, _, manifestSHA, _ := setupReleaseTagManifestRepo(t, manifestSpec{
		ReleaseBaseKind:     "tag",
		ReleaseBaseValue:    "v0.1.0",
		ReconciliationScope: "release arc from v0.1.0 through evaluated_commit",
		Records:             []manifestRecordSpec{overrideRequiredRecord("defer-ov", "v0.2.0")},
	})
	msgFile := filepath.Join(scratch, "msg.txt")
	if err := os.WriteFile(msgFile, []byte("release v0.2.0\n\n-test\n"), 0o644); err != nil {
		t.Fatalf("write msg: %v", err)
	}
	exitCode, result, _, stderr, _ := runReleaseTagManifest(t, wrapper, msgFile, "v0.2.0",
		[]string{"--override-release-version", "v0.2.0", "--override-manifest-sha", manifestSHA})
	if exitCode != 0 {
		t.Fatalf("valid override must ALLOW (exit 0); got %d", exitCode)
	}
	if !result.OK {
		t.Errorf("expected ok=true (error=%v)", result.Error)
	}
	if !tagExists(t, scratch, "v0.2.0") {
		t.Errorf("tag v0.2.0 must exist after accepted override")
	}
	if len(result.AcceptedOverrides) != 1 {
		t.Fatalf("accepted_overrides must have 1 entry; got %v", result.AcceptedOverrides)
	}
	ao := result.AcceptedOverrides[0]
	if ao["defer_id"] != "defer-ov" {
		t.Errorf("accepted_override.defer_id = defer-ov; got %v", ao["defer_id"])
	}
	if ao["approved_by"] != "operator-alice" {
		t.Errorf("accepted_override.approved_by must be disclosed; got %v", ao["approved_by"])
	}
	if !strings.Contains(stderr, "operator-alice") || !strings.Contains(stderr, "v0.2.0") {
		t.Errorf("stderr must print accepted override (approver + version) before tag; got:\n%s", stderr)
	}
}

// TestReleaseTag_Manifest_OverrideCannotCureStaleManifest — matrix case 25.
// The manifest has an unsupported schema_version (=2). The override ceremony
// flags are present and consistent, but the wrapper still REFUSES because the
// evaluator's schema check fails BEFORE the override check runs. The override
// cannot cure schema/staleness/ancestry/malformed failures.
func TestReleaseTag_Manifest_OverrideCannotCureStaleManifest(t *testing.T) {
	scratch, wrapper, _, manifestSHA, _ := setupReleaseTagManifestRepo(t, manifestSpec{
		ReleaseBaseKind:     "tag",
		ReleaseBaseValue:    "v0.1.0",
		ReconciliationScope: "release arc from v0.1.0 through evaluated_commit",
		Records:             []manifestRecordSpec{overrideRequiredRecord("defer-ov", "v0.2.0")},
		ForgedSchemaVersion: 2,
	})
	msgFile := filepath.Join(scratch, "msg.txt")
	if err := os.WriteFile(msgFile, []byte("release v0.2.0\n\n-test\n"), 0o644); err != nil {
		t.Fatalf("write msg: %v", err)
	}
	exitCode, result, _, _, _ := runReleaseTagManifest(t, wrapper, msgFile, "v0.2.0",
		[]string{"--override-release-version", "v0.2.0", "--override-manifest-sha", manifestSHA})
	if exitCode != 2 {
		t.Fatalf("stale manifest with override must REFUSE (exit 2 evaluator-error); got %d", exitCode)
	}
	if result.OK {
		t.Errorf("expected ok=false")
	}
	if tagExists(t, scratch, "v0.2.0") {
		t.Errorf("tag must NOT exist when manifest is schema-invalid")
	}
	if result.Error == nil || !strings.Contains(*result.Error, "evaluator-error") {
		t.Errorf("error must mention evaluator-error; got %v", result.Error)
	}
}

// TestReleaseTag_Manifest_RefusalsOccurBeforeTagMutation — matrix case 26.
// Manifest-mode refusals (blocker, evaluator-error, ceremony failure) ALL
// occur before `git tag -a`. Use a bare remote to prove the push path is also
// not reached.
func TestReleaseTag_Manifest_RefusalsOccurBeforeTagMutation(t *testing.T) {
	cases := []struct {
		name      string
		spec      manifestSpec
		version   string
		extraArgs []string
	}{
		{
			name: "blocker_record",
			spec: manifestSpec{
				ReleaseBaseKind:     "tag",
				ReleaseBaseValue:    "v0.1.0",
				ReconciliationScope: "release arc from v0.1.0 through evaluated_commit",
				Records: []manifestRecordSpec{{
					DeferID:          "defer-block",
					ReleaseRelevance: "yes",
					Disposition:      "block",
					MetadataState:    "valid",
					Summary:          "release-block",
					Reason:           "hard block",
					SourceRef:        ".local/x",
					StudiedAt:        "2026-07-15T00:00:00Z",
					ReviewedAt:       "2026-07-20T00:00:00Z",
				}},
			},
			version: "v0.2.0",
		},
		{
			name: "ceremony_version_mismatch",
			spec: manifestSpec{
				ReleaseBaseKind:     "tag",
				ReleaseBaseValue:    "v0.1.0",
				ReconciliationScope: "release arc from v0.1.0 through evaluated_commit",
				Records:             []manifestRecordSpec{overrideRequiredRecord("defer-ov", "v0.2.0")},
			},
			version:   "v0.2.0",
			extraArgs: []string{"--override-release-version", "v0.99.0", "--override-manifest-sha", strings.Repeat("0", 40)},
		},
		{
			name: "evaluator_error_schema",
			spec: manifestSpec{
				ReleaseBaseKind:     "tag",
				ReleaseBaseValue:    "v0.1.0",
				ReconciliationScope: "release arc from v0.1.0 through evaluated_commit",
				Records:             []manifestRecordSpec{seededNoDiscloseInvalid("defer-x")},
				ForgedSchemaVersion: 2,
			},
			version: "v0.2.0",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			scratch, wrapper, _, _, _ := setupReleaseTagManifestRepo(t, tc.spec)
			// Add a bare remote to detect any push attempt.
			remote := filepath.Join(t.TempDir(), "remote.git")
			rcmd := func(args ...string) {
				t.Helper()
				out, err := exec.Command("git", args...).CombinedOutput()
				if err != nil {
					t.Fatalf("git %v: %v\n%s", args, err, out)
				}
			}
			rcmd("init", "--bare", "-q", remote)
			rcmd("-C", scratch, "remote", "add", "origin", remote)

			msgFile := filepath.Join(scratch, "msg.txt")
			if err := os.WriteFile(msgFile, []byte("release v0.2.0\n\n-test\n"), 0o644); err != nil {
				t.Fatalf("write msg: %v", err)
			}
			// RELEASE_TAG_PUSH=1 to also assert push path is not reached.
			// Run from <scratch> (repo root) — the wrapper references
			// .opencode/scripts/... and .vh-agent-harness/... relative to root.
			cmd := exec.Command("bash", append([]string{wrapper, tc.version}, tc.extraArgs...)...)
			cmd.Dir = filepath.Dir(filepath.Dir(wrapper))
			cmd.Env = append(os.Environ(),
				"RELEASE_TAG_MESSAGE_FILE="+msgFile,
				"RELEASE_TAG_PUSH=1",
			)
			out, err := cmd.CombinedOutput()
			exitCode := 0
			if err != nil {
				var exitErr *exec.ExitError
				if errors.As(err, &exitErr) {
					exitCode = exitErr.ExitCode()
				} else {
					t.Fatalf("bash spawn error: %v\n%s", err, out)
				}
			}
			if exitCode == 0 {
				t.Fatalf("%s: must REFUSE (nonzero); got exit 0\n%s", tc.name, out)
			}
			if tagExists(t, scratch, tc.version) {
				t.Errorf("%s: tag %s must NOT exist after refusal", tc.name, tc.version)
			}
			// Remote must have ZERO tags — push path not reached.
			rout, err := exec.Command("git", "-C", remote, "tag", "-l").Output()
			if err != nil {
				t.Fatalf("git -C remote tag -l: %v", err)
			}
			if strings.TrimSpace(string(rout)) != "" {
				t.Errorf("%s: remote must have NO tags (push path not reached); got %q", tc.name, string(rout))
			}
		})
	}
}

// TestReleaseTag_Manifest_MissingManifestRefusesAfterActivation — matrix case 3
// at the wrapper level. Manifest mode active + no committed manifest → the
// evaluator refuses (exit 2), the wrapper propagates the refusal, no tag is
// created.
func TestReleaseTag_Manifest_MissingManifestRefusesAfterActivation(t *testing.T) {
	// Reuse setupReleaseTagRepo (which does NOT seed a manifest) so we can
	// verify the missing-manifest refusal at the wrapper level.
	scratch, wrapper, _, msgFile := setupReleaseTagRepo(t)
	cmd := exec.Command("bash", wrapper, "v0.2.0")
	cmd.Dir = filepath.Dir(filepath.Dir(wrapper))
	cmd.Env = append(os.Environ(),
		"RELEASE_TAG_MESSAGE_FILE="+msgFile,
	)
	out, err := cmd.CombinedOutput()
	exitCode := 0
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			t.Fatalf("bash spawn error: %v\n%s", err, out)
		}
	}
	if exitCode != 2 {
		t.Fatalf("missing manifest with authority active must REFUSE (exit 2); got %d\n%s", exitCode, out)
	}
	if tagExists(t, scratch, "v0.2.0") {
		t.Errorf("tag must NOT exist when manifest is missing")
	}
}

// TestReleaseTag_Manifest_DirtyWorktreeManifestRefusesTag — pins the
// committed-blob authority contract at the wrapper level. A committed `block`
// record MUST refuse the tag even when the worktree file is dirtily flipped
// to `disclose+valid`, because the evaluator reads bytes from HEAD (not the
// worktree) and the wrapper computes the override ceremony SHA from
// `git rev-parse HEAD:<path>` (not `git hash-object <path>`).
//
// Scenario:
//  1. Commit a manifest with defer-dirty = `yes+block+valid` (full handshake
//     satisfied). Committed state blocks; no override present so the operator
//     ceremony is not engaged.
//  2. Dirty the worktree: flip the record to `yes+disclose+valid` UNCOMMITTED.
//     Handshake SHAs stay intact so the only divergence is the record body.
//  3. Invoke the wrapper with manifest mode active, attempting to tag.
//
// Before the F1 fix: the evaluator read the dirty `disclose+valid` from the
// worktree, classified `disclose` (exit 0), the wrapper created the tag, and
// a gate-protected release tag existed on a commit whose committed manifest
// blocks. CI's fresh-checkout recheck would later refuse publication, but the
// tag was already created.
//
// After the F1 fix: the evaluator reads the committed `block` from HEAD, the
// wrapper propagates the refusal (exit 1, blocker), and no tag is created.
func TestReleaseTag_Manifest_DirtyWorktreeManifestRefusesTag(t *testing.T) {
	scratch, wrapper, manifestPath, _, _ := setupReleaseTagManifestRepo(t, manifestSpec{
		ReleaseBaseKind:     "tag",
		ReleaseBaseValue:    "v0.1.0",
		ReconciliationScope: "release arc from v0.1.0 through evaluated_commit",
		Records: []manifestRecordSpec{
			{
				DeferID:          "defer-dirty",
				ReleaseRelevance: "yes",
				Disposition:      "block",
				MetadataState:    "valid",
				Summary:          "release-block (committed)",
				Reason:           "hard block from committed state",
				SourceRef:        ".local/x",
				StudiedAt:        "2026-07-15T00:00:00Z",
				ReviewedAt:       "2026-07-20T00:00:00Z",
			},
		},
	})

	// 1. Dirty the worktree: flip the committed `block` to `disclose` UNCOMMITTED.
	//    Handshake SHAs stay intact so the bypass scenario is precisely the
	//    record-body divergence.
	committedBytes, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read committed manifest: %v", err)
	}
	flipped := strings.ReplaceAll(string(committedBytes), `"disposition": "block"`, `"disposition": "disclose"`)
	if flipped == string(committedBytes) {
		t.Fatalf("setup invariant: flip block→disclose did not change manifest bytes; raw=\n%s", string(committedBytes))
	}
	if err := os.WriteFile(manifestPath, []byte(flipped), 0o644); err != nil {
		t.Fatalf("write dirty worktree manifest: %v", err)
	}

	// 2. Sanity: verify the worktree is dirty relative to HEAD.
	statusCmd := exec.Command("git", "-C", scratch, "status", "--porcelain", "--", ".vh-agent-harness/release-defer-dispositions.json")
	statusOut, err := statusCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git status --porcelain: %v\n%s", err, statusOut)
	}
	if strings.TrimSpace(string(statusOut)) == "" {
		t.Fatalf("setup invariant: worktree manifest must be dirty relative to HEAD")
	}

	// 3. Invoke the wrapper with manifest mode active. No override flags — the
	//    committed record is a hard block, not override_required.
	msgFile := filepath.Join(scratch, "msg.txt")
	if err := os.WriteFile(msgFile, []byte("release v0.2.0\n\n-test\n"), 0o644); err != nil {
		t.Fatalf("write msg: %v", err)
	}
	exitCode, result, _, _, _ := runReleaseTagManifest(t, wrapper, msgFile, "v0.2.0", nil)

	// 4. Assert: REFUSAL — the evaluator read the committed `block` from HEAD,
	//    not the dirty worktree's `disclose+valid`. No tag is created.
	if exitCode == 0 {
		t.Fatalf("dirty-worktree bypass must REFUSE (nonzero); got exit 0 (ok=%v error=%v)", result.OK, result.Error)
	}
	if result.OK {
		t.Errorf("expected ok=false; got true")
	}
	if tagExists(t, scratch, "v0.2.0") {
		t.Errorf("tag v0.2.0 must NOT exist after dirty-worktree refusal")
	}
	// The error must carry the committed blocking ID, proving the evaluator
	// read HEAD (block) rather than the worktree (disclose).
	if result.Error == nil || !strings.Contains(*result.Error, "defer-dirty") {
		t.Errorf("error must carry committed blocking ID defer-dirty; got %v", result.Error)
	}
	// Disclosures must be EMPTY — the committed record blocks, it is not
	// disclosed. (If the evaluator had read the worktree, disclosures would
	// contain defer-dirty.)
	if len(result.Disclosures) != 0 {
		t.Errorf("committed block must produce ZERO disclosures (worktree disclose+valid must be ignored); got %v", result.Disclosures)
	}
}

// TestReleaseTag_Manifest_DirtyWorktreeOverrideCeremonyBindsToCommittedBlob —
// complementary to the block-record test: even when the operator runs the
// override ceremony, the SHA the wrapper compares --override-manifest-sha
// against is `git rev-parse HEAD:<path>`, NOT `git hash-object <path>`. So a
// dirty worktree cannot swap the SHA under the ceremony either.
//
// Scenario:
//  1. Commit a manifest with defer-ov = `yes+override_required+valid` and an
//     override.release_version matching the version being tagged.
//  2. Compute the operator's --override-manifest-sha from the COMMITTED blob
//     (`git rev-parse HEAD:<path>` — what the wrapper now uses).
//  3. Dirty the worktree: rewrite the manifest file (uncommitted) so the
//     worktree blob SHA differs from the committed blob SHA.
//  4. Invoke the wrapper with the ceremony flags bound to the COMMITTED SHA.
//
// Before the F1 fix: the wrapper ran `git hash-object` (worktree SHA), which
// no longer matched the operator's committed-blob SHA → ceremony refused
// (good outcome by accident, but for the wrong reason — the wrapper was
// reading the worktree).
//
// After the F1 fix: the wrapper runs `git rev-parse HEAD:<path>` (committed
// SHA), which matches the operator's committed-blob SHA → ceremony succeeds,
// override is forwarded, evaluator accepts (reading the committed
// override_required record from HEAD), and the tag is created. This proves
// the ceremony now binds coherently to the committed blob.
func TestReleaseTag_Manifest_DirtyWorktreeOverrideCeremonyBindsToCommittedBlob(t *testing.T) {
	scratch, wrapper, manifestPath, _, _ := setupReleaseTagManifestRepo(t, manifestSpec{
		ReleaseBaseKind:     "tag",
		ReleaseBaseValue:    "v0.1.0",
		ReconciliationScope: "release arc from v0.1.0 through evaluated_commit",
		Records:             []manifestRecordSpec{overrideRequiredRecord("defer-ov", "v0.2.0")},
	})

	// 1. Compute the operator's --override-manifest-sha from the COMMITTED
	//    blob (git rev-parse HEAD:<path>) — what an operator would derive
	//    from `git show HEAD:.vh-agent-harness/...` or `cat-file blob`.
	committedSHA, err := exec.Command("git", "-C", scratch, "rev-parse", "HEAD:.vh-agent-harness/release-defer-dispositions.json").Output()
	if err != nil {
		t.Fatalf("git rev-parse HEAD:<manifest>: %v", err)
	}
	committedSHAStr := strings.TrimSpace(string(committedSHA))

	// 2. Dirty the worktree: rewrite the manifest file so its worktree blob
	//    SHA differs from the committed blob SHA. Keep the override object
	//    intact (the dirty edit is to the unrelated summary text).
	dirtyBytes, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read committed manifest: %v", err)
	}
	dirtied := strings.ReplaceAll(string(dirtyBytes), "Operator may accept residual risk for this release version.",
		"Operator may accept residual risk for this release version. (dirty worktree edit)")
	if dirtied == string(dirtyBytes) {
		t.Fatalf("setup invariant: dirty edit did not change manifest bytes")
	}
	if err := os.WriteFile(manifestPath, []byte(dirtied), 0o644); err != nil {
		t.Fatalf("write dirty worktree manifest: %v", err)
	}

	// 3. Sanity: the worktree blob SHA must differ from the committed blob SHA.
	worktreeSHA, err := exec.Command("git", "-C", scratch, "hash-object", ".vh-agent-harness/release-defer-dispositions.json").Output()
	if err != nil {
		t.Fatalf("git hash-object worktree: %v", err)
	}
	if strings.TrimSpace(string(worktreeSHA)) == committedSHAStr {
		t.Fatalf("setup invariant: worktree blob SHA must differ from committed blob SHA")
	}

	// 4. Invoke the wrapper with the ceremony bound to the COMMITTED blob SHA.
	//    After the F1 fix the wrapper also derives ACTUAL_SHA from the
	//    committed blob, so the ceremony matches and the tag is created.
	msgFile := filepath.Join(scratch, "msg.txt")
	if err := os.WriteFile(msgFile, []byte("release v0.2.0\n\n-test\n"), 0o644); err != nil {
		t.Fatalf("write msg: %v", err)
	}
	exitCode, result, _, _, _ := runReleaseTagManifest(t, wrapper, msgFile, "v0.2.0",
		[]string{"--override-release-version", "v0.2.0", "--override-manifest-sha", committedSHAStr})

	if exitCode != 0 {
		t.Fatalf("dirty-worktree override must ALLOW with committed-SHA ceremony (exit 0); got %d (ok=%v err=%v)",
			exitCode, result.OK, result.Error)
	}
	if !result.OK {
		t.Errorf("expected ok=true; got false (error=%v)", result.Error)
	}
	if !tagExists(t, scratch, "v0.2.0") {
		t.Errorf("tag v0.2.0 must exist after accepted override")
	}
	if len(result.AcceptedOverrides) != 1 {
		t.Errorf("accepted_overrides must have 1 entry; got %v", result.AcceptedOverrides)
	}
}
