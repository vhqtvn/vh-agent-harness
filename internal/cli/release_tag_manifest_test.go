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
	// Seed a buildable, gofmt-stable Go module so the wrapper's G0
	// green-tree gate (go test/vet/build/gofmt) passes by default. The
	// package name is non-main so no `func main()` is required.
	writeFile("go.mod", "module scratch\n\ngo 1.21\n")
	writeFile("fileA.go", "package scratch\n")
	writeFile("fileB.go", "package scratch\n")
	writeFile("dir/fileC.go", "package scratch\n")
	git("add", "-A")
	git("commit", "-q", "-m", "initial")
	git("tag", "v0.1.0")
	// Post-tag changes (in the arc v0.1.0..HEAD): fileA.go + dir/fileC.go.
	// Use declarations (not bare comments) so gofmt -l stays clean.
	writeFile("fileA.go", "package scratch\n\n// FileAChanged marks the arc commit.\nconst FileAChanged = true\n")
	writeFile("dir/fileC.go", "package scratch\n\n// FileCChanged marks the arc commit.\nconst FileCChanged = true\n")
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
	spec := manifestSpec{
		ReleaseBaseKind:     "tag",
		ReleaseBaseValue:    "v0.1.0",
		ReconciliationScope: "release arc from v0.1.0 through evaluated_commit",
		Records:             []manifestRecordSpec{seededNoDiscloseInvalid("defer-seed")},
	}
	scratch, wrapper, _, _, _ := setupReleaseTagManifestRepo(t, spec)
	// Phase C: insert a valid readiness artifact so the readiness gate passes.
	insertReadinessArtifactCommit(t, scratch, spec, readinessArtifactSpec{})
	msgFile := filepath.Join(t.TempDir(), "msg.txt")
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
	msgFile := filepath.Join(t.TempDir(), "msg.txt")
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
	msgFile := filepath.Join(t.TempDir(), "msg.txt")
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
	msgFile := filepath.Join(t.TempDir(), "msg.txt")
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
	spec := manifestSpec{
		ReleaseBaseKind:     "tag",
		ReleaseBaseValue:    "v0.1.0",
		ReconciliationScope: "release arc from v0.1.0 through evaluated_commit",
		Records:             []manifestRecordSpec{overrideRequiredRecord("defer-ov", "v0.2.0")},
	}
	scratch, wrapper, _, _, _ := setupReleaseTagManifestRepo(t, spec)
	// Phase C: insert a valid readiness artifact. This re-stamps the manifest,
	// so the operator's ceremony SHA must be recomputed from the re-stamped blob.
	manifestSHA := insertReadinessArtifactCommit(t, scratch, spec, readinessArtifactSpec{})
	msgFile := filepath.Join(t.TempDir(), "msg.txt")
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
	msgFile := filepath.Join(t.TempDir(), "msg.txt")
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

			msgFile := filepath.Join(t.TempDir(), "msg.txt")
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
	msgFile := filepath.Join(t.TempDir(), "msg.txt")
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

// TestReleaseTag_Manifest_DirtyWorktreeRefusedByG0bEvenWithOverrideCeremony —
// the wrapper's deterministic G0b gate (clean-worktree check, added in the
// Phase A release-readiness enforcement) now fires BEFORE the override
// ceremony can take effect: a dirty worktree is refused at G0b even when
// the operator supplies consistent --override-release-version +
// --override-manifest-sha flags that would otherwise bind to the committed
// manifest blob. This closes the dirty-worktree bypass class at the wrapper
// level — uncommitted edits cannot reach the tag mutation.
//
// Historical note: this test was originally
// DirtyWorktreeOverrideCeremonyBindsToCommittedBlob, verifying the F1 fix
// that switched the ceremony's SHA comparison from `git hash-object` (worktree
// blob) to `git rev-parse HEAD:<path>` (committed blob). The F1 fix is still
// in the code and is still tested at the EVALUATOR level
// (TestReleaseEval_DirtyWorktreeManifestReadsCommittedHEAD in
// check_defer_release_manifest_test.go). At the WRAPPER level, G0b now fires
// first on any dirty worktree, so the F1 ceremony binding is no longer the
// relevant defense — G0b is strictly stronger (prevents the dirty worktree
// from reaching the ceremony at all).
//
// Scenario:
//  1. Commit a manifest with defer-ov = `yes+override_required+valid` and an
//     override.release_version matching the version being tagged.
//  2. Compute the operator's --override-manifest-sha from the COMMITTED blob
//     (`git rev-parse HEAD:<path>`).
//  3. Dirty the worktree: rewrite the manifest file (uncommitted) so the
//     worktree blob SHA differs from the committed blob SHA.
//  4. Invoke the wrapper with the ceremony flags bound to the COMMITTED SHA.
//
// Expected (post-G0b): G0b refuses because the worktree is dirty. The tag
// is NOT created. The ceremony flags are irrelevant — G0b fires before the
// ceremony matters.
func TestReleaseTag_Manifest_DirtyWorktreeRefusedByG0bEvenWithOverrideCeremony(t *testing.T) {
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
	//    G0b must refuse because the worktree is dirty, regardless of the
	//    ceremony's validity.
	msgFile := filepath.Join(t.TempDir(), "msg.txt")
	if err := os.WriteFile(msgFile, []byte("release v0.2.0\n\n-test\n"), 0o644); err != nil {
		t.Fatalf("write msg: %v", err)
	}
	exitCode, result, _, _, _ := runReleaseTagManifest(t, wrapper, msgFile, "v0.2.0",
		[]string{"--override-release-version", "v0.2.0", "--override-manifest-sha", committedSHAStr})

	if exitCode == 0 {
		t.Fatalf("dirty worktree must be REFUSED by G0b (nonzero); got exit 0 (ok=%v error=%v)",
			result.OK, result.Error)
	}
	if result.OK {
		t.Errorf("expected ok=false; got true")
	}
	if !tagExists(t, scratch, "v0.2.0") {
		// expected — tag must NOT be created
	} else {
		t.Errorf("tag v0.2.0 must NOT exist after G0b dirty-worktree refusal")
	}
	// The error must mention G0b dirty worktree, proving G0b fired (not the
	// ceremony gate, not the DEFER gate, not the manifest evaluator).
	if result.Error == nil || !strings.Contains(*result.Error, "G0b") {
		t.Errorf("error must mention G0b dirty worktree; got %v", result.Error)
	}
	if result.Error == nil || !strings.Contains(*result.Error, "dirty worktree") {
		t.Errorf("error must mention dirty worktree; got %v", result.Error)
	}
}

// =============================================================================
// DETERMINISTIC GATE TESTS — G0 (green tree) / G0b (clean worktree)
//
// The wrapper now independently recomputes the deterministic readiness gates
// at tag time (Phase A of release-boundary enforcement). These tests verify
// each gate's pass and refuse behavior. The green-path (G0+G0b pass → tag
// created) is already covered by TestReleaseTag_Manifest_NoDiscloseRecord_Allows
// which runs the full wrapper including the G0/G0b block on the buildable
// seed; the tests below cover the REFUSE side.
//
// IMPORTANT: the DEFER gate runs BEFORE G0/G0b. To make G0 refuse tests
// reach the G0 gate, the manifest must be re-stamped (fresh handshake)
// AFTER committing the broken/unformatted file so the DEFER gate passes.
// =============================================================================

// commitScratchFile writes content to rel within scratch and commits it.
// Used by G0 refuse tests to introduce broken/unformatted Go files into
// the committed tree (so G0 sees them at tag time).
func commitScratchFile(t *testing.T, scratch, rel, content, msg string) {
	t.Helper()
	full := filepath.Join(scratch, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", rel, err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", rel, err)
	}
	cmd := exec.Command("git", "-C", scratch, "add", rel)
	if err := cmd.Run(); err != nil {
		t.Fatalf("git add %s: %v", rel, err)
	}
	cmd = exec.Command("git", "-C", scratch, "commit", "-q", "-m", msg)
	if err := cmd.Run(); err != nil {
		t.Fatalf("git commit %s: %v", rel, err)
	}
}

// restampManifest re-commits the manifest with a fresh handshake (pointing
// at the current HEAD) so the DEFER gate passes after a test has committed
// additional files on top of the manifest commit. This creates a new
// immediate-child manifest commit at HEAD.
func restampManifest(t *testing.T, scratch string, spec manifestSpec) {
	t.Helper()
	manifestBytes := buildManifestBytes(t, scratch, spec)
	commitReleaseManifest(t, scratch, manifestBytes, "")
}

// TestReleaseTag_DeterministicGate_G0_BrokenBuildRefuses — a committed .go
// file with a syntax error causes G0 (go test ./...) to fail and the wrapper
// to refuse the tag BEFORE `git tag -a`. This is fail-closed matrix case 1
// (deterministic gate failure).
func TestReleaseTag_DeterministicGate_G0_BrokenBuildRefuses(t *testing.T) {
	spec := manifestSpec{
		ReleaseBaseKind:     "tag",
		ReleaseBaseValue:    "v0.1.0",
		ReconciliationScope: "release arc from v0.1.0 through evaluated_commit",
		Records:             []manifestRecordSpec{seededNoDiscloseInvalid("defer-seed")},
	}
	scratch, wrapper, _, _, _ := setupReleaseTagManifestRepo(t, spec)
	// Commit a broken .go file (syntax error) so G0 go test ./... fails.
	commitScratchFile(t, scratch, "broken.go", "package scratch\n\nfunc broken( {}\n", "broken")
	// Re-stamp the manifest so the DEFER gate passes and G0 gets to run.
	restampManifest(t, scratch, spec)

	msgFile := filepath.Join(t.TempDir(), "msg.txt")
	if err := os.WriteFile(msgFile, []byte("release v0.2.0\n\n-test\n"), 0o644); err != nil {
		t.Fatalf("write msg: %v", err)
	}
	exitCode, result, _, _, _ := runReleaseTagManifest(t, wrapper, msgFile, "v0.2.0", nil)

	if exitCode == 0 {
		t.Fatalf("broken build must be REFUSED by G0 (nonzero); got exit 0")
	}
	if result.OK {
		t.Errorf("expected ok=false; got true")
	}
	if tagExists(t, scratch, "v0.2.0") {
		t.Errorf("tag v0.2.0 must NOT exist after G0 refusal")
	}
	// The error must mention G0 and the failing command.
	if result.Error == nil || !strings.Contains(*result.Error, "G0") {
		t.Errorf("error must mention G0; got %v", result.Error)
	}
}

// TestReleaseTag_DeterministicGate_G0_GofmtRefuses — a committed .go file
// that is not gofmt-formatted causes G0 (gofmt -l .) to report it and the
// wrapper to refuse. The file compiles fine (go test/vet/build pass) but
// fails the formatting check. This is fail-closed matrix case 1.
func TestReleaseTag_DeterministicGate_G0_GofmtRefuses(t *testing.T) {
	spec := manifestSpec{
		ReleaseBaseKind:     "tag",
		ReleaseBaseValue:    "v0.1.0",
		ReconciliationScope: "release arc from v0.1.0 through evaluated_commit",
		Records:             []manifestRecordSpec{seededNoDiscloseInvalid("defer-seed")},
	}
	scratch, wrapper, _, _, _ := setupReleaseTagManifestRepo(t, spec)
	// Commit a valid but unformatted .go file. go test/vet/build pass;
	// gofmt -l reports it.
	commitScratchFile(t, scratch, "ugly.go", "package scratch\n\ntype T struct{x int}\n", "ugly")
	// Re-stamp the manifest so the DEFER gate passes and G0 gets to run.
	restampManifest(t, scratch, spec)

	msgFile := filepath.Join(t.TempDir(), "msg.txt")
	if err := os.WriteFile(msgFile, []byte("release v0.2.0\n\n-test\n"), 0o644); err != nil {
		t.Fatalf("write msg: %v", err)
	}
	exitCode, result, _, _, _ := runReleaseTagManifest(t, wrapper, msgFile, "v0.2.0", nil)

	if exitCode == 0 {
		t.Fatalf("unformatted file must be REFUSED by G0 (nonzero); got exit 0")
	}
	if result.OK {
		t.Errorf("expected ok=false; got true")
	}
	if tagExists(t, scratch, "v0.2.0") {
		t.Errorf("tag v0.2.0 must NOT exist after G0 refusal")
	}
	if result.Error == nil || !strings.Contains(*result.Error, "G0") {
		t.Errorf("error must mention G0; got %v", result.Error)
	}
	if result.Error == nil || !strings.Contains(*result.Error, "gofmt") {
		t.Errorf("error must mention gofmt; got %v", result.Error)
	}
}

// TestReleaseTag_DeterministicGate_G0b_DirtyWorktreeRefuses — an uncommitted
// modification causes G0b (git status --short) to report it and the wrapper
// to refuse. This is fail-closed matrix case 2 (dirty tree).
//
// Note: this test does NOT re-stamp the manifest because the dirty edit is
// uncommitted — the manifest at HEAD is already fresh. The DEFER gate reads
// committed state (HEAD) and passes; G0 passes on the buildable seed; G0b
// catches the uncommitted modification.
func TestReleaseTag_DeterministicGate_G0b_DirtyWorktreeRefuses(t *testing.T) {
	scratch, wrapper, _, _, _ := setupReleaseTagManifestRepo(t, manifestSpec{
		ReleaseBaseKind:     "tag",
		ReleaseBaseValue:    "v0.1.0",
		ReconciliationScope: "release arc from v0.1.0 through evaluated_commit",
		Records:             []manifestRecordSpec{seededNoDiscloseInvalid("defer-seed")},
	})
	// Dirty the worktree WITHOUT committing: modify an existing tracked file.
	dirtyPath := filepath.Join(scratch, "fileB.go")
	if err := os.WriteFile(dirtyPath, []byte("package scratch\n\n// dirty uncommitted edit\nconst DirtyEdit = true\n"), 0o644); err != nil {
		t.Fatalf("write dirty fileB.go: %v", err)
	}

	msgFile := filepath.Join(t.TempDir(), "msg.txt")
	if err := os.WriteFile(msgFile, []byte("release v0.2.0\n\n-test\n"), 0o644); err != nil {
		t.Fatalf("write msg: %v", err)
	}
	exitCode, result, _, _, _ := runReleaseTagManifest(t, wrapper, msgFile, "v0.2.0", nil)

	if exitCode == 0 {
		t.Fatalf("dirty worktree must be REFUSED by G0b (nonzero); got exit 0")
	}
	if result.OK {
		t.Errorf("expected ok=false; got true")
	}
	if tagExists(t, scratch, "v0.2.0") {
		t.Errorf("tag v0.2.0 must NOT exist after G0b refusal")
	}
	if result.Error == nil || !strings.Contains(*result.Error, "G0b") {
		t.Errorf("error must mention G0b; got %v", result.Error)
	}
	if result.Error == nil || !strings.Contains(*result.Error, "dirty worktree") {
		t.Errorf("error must mention dirty worktree; got %v", result.Error)
	}
}

// =============================================================================
// RELEASE READINESS-PASS ARTIFACT GATE (G1-G5, model-driven) — Phase C
// =============================================================================
//
// Phase C activates the readiness-artifact requirement. After Phase C, every
// release requires: deterministic gates pass (G0/G0b, Phase A) AND DEFER gate
// passes (G7, pre-existing) AND readiness artifact exists at
// HEAD:.vh-agent-harness/release-readiness-pass.json with all five model
// gates (G1-G5) reporting "ready" and commit_sha pinned to HEAD^^.
//
// The readiness ceremony sequences: note → artifact → manifest. At tag time
// HEAD is the manifest commit (DEFER handshake intact), HEAD^ is the artifact
// commit, HEAD^^ is the release-prep commit the readiness agent evaluated.

// readinessArtifactSpec controls the content of the readiness-pass artifact.
// By default it produces a valid schema-v1 artifact with all five gates
// "ready" and commit_sha pinned to the release-prep commit. Fields like
// RawJSON or OmitField let tests forge invalid artifacts for the fail-closed
// matrix.
type readinessArtifactSpec struct {
	// CommitSHA overrides commit_sha. If empty, uses the release-prep SHA
	// (HEAD^^ at tag time). Set to a wrong 40-hex value to test the binding.
	CommitSHA string

	// Gates overrides individual gate verdicts. If nil, all five default
	// to "ready". Use to set specific gates to "blocked" or "skipped" or
	// an unknown value.
	Gates map[string]string

	// RawJSON, if non-empty, is written verbatim (bypasses the builder).
	// Use for malformed-JSON tests.
	RawJSON string

	// OmitField, if non-empty, removes the named top-level field from the
	// built JSON (e.g. "schema_version", "commit_sha", "model_gates").
	OmitField string

	// OmitGate, if non-empty, removes the named gate from model_gates.
	OmitGate string
}

// buildReadinessArtifactBytes builds the schema-v1 readiness-pass artifact
// JSON from the spec. If spec.RawJSON is non-empty, it is returned verbatim.
func buildReadinessArtifactBytes(t *testing.T, spec readinessArtifactSpec) []byte {
	t.Helper()
	if spec.RawJSON != "" {
		return []byte(spec.RawJSON)
	}
	gates := map[string]string{
		"G1_coverage":     "ready",
		"G2_significance": "ready",
		"G3_docs":         "ready",
		"G4_visibility":   "ready",
		"G5_curated_note": "ready",
	}
	for k, v := range spec.Gates {
		gates[k] = v
	}
	if spec.OmitGate != "" {
		delete(gates, spec.OmitGate)
	}
	obj := map[string]interface{}{
		"schema_version": 1,
		"commit_sha":     spec.CommitSHA,
		"model_gates":    gates,
	}
	if spec.OmitField != "" {
		delete(obj, spec.OmitField)
	}
	b, err := json.Marshal(obj)
	if err != nil {
		t.Fatalf("marshal readiness artifact: %v", err)
	}
	return b
}

// insertReadinessArtifactCommit inserts a readiness-pass artifact commit as
// HEAD^ and re-stamps the manifest as HEAD. The release-prep commit
// (previously HEAD^) becomes HEAD^^.
//
// Before: HEAD=manifest, HEAD^=release-prep
// After:  HEAD=manifest(re-stamped), HEAD^=artifact(single-path), HEAD^^=release-prep
//
// The artifact's commit_sha is set to the release-prep SHA unless the spec
// overrides it. Returns the re-stamped manifest blob SHA (for the override
// ceremony).
func insertReadinessArtifactCommit(t *testing.T, scratch string, mspec manifestSpec, aspec readinessArtifactSpec) string {
	t.Helper()

	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", scratch}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	// 1. Capture release-prep SHA (current HEAD^ = arc commit).
	out, err := exec.Command("git", "-C", scratch, "rev-parse", "HEAD^").Output()
	if err != nil {
		t.Fatalf("git rev-parse HEAD^: %v", err)
	}
	releasePrepSHA := strings.TrimSpace(string(out))

	// 2. Undo the manifest commit (soft reset keeps manifest staged).
	git("reset", "--soft", "HEAD~1")

	// 3. Unstage the manifest so the artifact commit is single-path.
	git("reset", "HEAD", "--", ".vh-agent-harness/release-defer-dispositions.json")

	// 4. Fill commit_sha if not overridden.
	if aspec.CommitSHA == "" {
		aspec.CommitSHA = releasePrepSHA
	}

	// 5. Write and commit the artifact.
	artifactBytes := buildReadinessArtifactBytes(t, aspec)
	artifactPath := filepath.Join(scratch, ".vh-agent-harness", "release-readiness-pass.json")
	if err := os.MkdirAll(filepath.Dir(artifactPath), 0o755); err != nil {
		t.Fatalf("mkdir artifact dir: %v", err)
	}
	if err := os.WriteFile(artifactPath, artifactBytes, 0o644); err != nil {
		t.Fatalf("write artifact: %v", err)
	}
	git("add", ".vh-agent-harness/release-readiness-pass.json")
	git("commit", "-q", "-m", "release-readiness artifact")

	// 6. Re-stamp the manifest (evaluated_commit = HEAD = artifact commit).
	restampManifest(t, scratch, mspec)

	// 7. Return the re-stamped manifest blob SHA for the override ceremony.
	out, err = exec.Command("git", "-C", scratch, "hash-object", ".vh-agent-harness/release-defer-dispositions.json").Output()
	if err != nil {
		t.Fatalf("hash-object manifest after re-stamp: %v", err)
	}
	return strings.TrimSpace(string(out))
}

// manifestSpecForReadiness is a shorthand for the common manifest spec used
// by readiness tests: a single no-disclose record over the v0.1.0 → v0.2.0 arc.
func manifestSpecForReadiness() manifestSpec {
	return manifestSpec{
		ReleaseBaseKind:     "tag",
		ReleaseBaseValue:    "v0.1.0",
		ReconciliationScope: "release arc from v0.1.0 through evaluated_commit",
		Records:             []manifestRecordSpec{seededNoDiscloseInvalid("defer-seed")},
	}
}

// runReleaseTagFromScratch is a shorthand for the common wrapper invocation
// used by readiness tests: write a msg file OUTSIDE the repo, run the wrapper
// for v0.2.0, return the result.
func runReleaseTagFromScratch(t *testing.T, wrapper string) (int, releaseTagManifestResult) {
	t.Helper()
	msgFile := filepath.Join(t.TempDir(), "msg.txt")
	if err := os.WriteFile(msgFile, []byte("release v0.2.0\n\n-test\n"), 0o644); err != nil {
		t.Fatalf("write msg: %v", err)
	}
	exitCode, result, _, _, _ := runReleaseTagManifest(t, wrapper, msgFile, "v0.2.0", nil)
	return exitCode, result
}

// --- GREEN PATH ---

// TestReleaseTag_ReadinessArtifact_GreenPath_Allows — valid artifact with
// all five gates "ready", deterministic gates pass, DEFER passes → tag created.
func TestReleaseTag_ReadinessArtifact_GreenPath_Allows(t *testing.T) {
	scratch, wrapper, _, _, _ := setupReleaseTagManifestRepo(t, manifestSpecForReadiness())
	insertReadinessArtifactCommit(t, scratch, manifestSpecForReadiness(), readinessArtifactSpec{})

	exitCode, result := runReleaseTagFromScratch(t, wrapper)
	if exitCode != 0 {
		t.Fatalf("green-path readiness must ALLOW (exit 0); got %d (error=%v)", exitCode, result.Error)
	}
	if !result.OK {
		t.Errorf("expected ok=true; got false (error=%v)", result.Error)
	}
	if !tagExists(t, scratch, "v0.2.0") {
		t.Errorf("tag v0.2.0 must exist after green-path readiness")
	}
}

// --- FAIL-CLOSED MATRIX (cases 4-9; cases 1-3 covered by Phase A) ---

// Case 4: missing artifact at HEAD: → refuse.
func TestReleaseTag_ReadinessArtifact_MissingArtifactRefuses(t *testing.T) {
	scratch, wrapper, _, _, _ := setupReleaseTagManifestRepo(t, manifestSpecForReadiness())
	// No readiness artifact committed.

	exitCode, result := runReleaseTagFromScratch(t, wrapper)
	if exitCode == 0 {
		t.Fatalf("missing readiness artifact must REFUSE (nonzero); got exit 0")
	}
	if result.OK {
		t.Errorf("expected ok=false; got true")
	}
	if tagExists(t, scratch, "v0.2.0") {
		t.Errorf("tag v0.2.0 must NOT exist after missing-artifact refusal")
	}
	if result.Error == nil || !strings.Contains(*result.Error, "missing readiness artifact") {
		t.Errorf("error must mention missing readiness artifact; got %v", result.Error)
	}
}

// Case 5a: malformed JSON → refuse.
func TestReleaseTag_ReadinessArtifact_MalformedJSONRefuses(t *testing.T) {
	scratch, wrapper, _, _, _ := setupReleaseTagManifestRepo(t, manifestSpecForReadiness())
	insertReadinessArtifactCommit(t, scratch, manifestSpecForReadiness(), readinessArtifactSpec{
		RawJSON: `{not valid json`,
	})

	exitCode, result := runReleaseTagFromScratch(t, wrapper)
	if exitCode == 0 {
		t.Fatalf("malformed JSON must REFUSE; got exit 0")
	}
	if result.OK {
		t.Errorf("expected ok=false; got true")
	}
	if result.Error == nil || !strings.Contains(*result.Error, "not valid JSON") {
		t.Errorf("error must mention invalid JSON; got %v", result.Error)
	}
}

// Case 5b: missing schema_version → refuse.
func TestReleaseTag_ReadinessArtifact_MissingSchemaVersionRefuses(t *testing.T) {
	scratch, wrapper, _, _, _ := setupReleaseTagManifestRepo(t, manifestSpecForReadiness())
	insertReadinessArtifactCommit(t, scratch, manifestSpecForReadiness(), readinessArtifactSpec{
		OmitField: "schema_version",
	})

	exitCode, result := runReleaseTagFromScratch(t, wrapper)
	if exitCode == 0 {
		t.Fatalf("missing schema_version must REFUSE; got exit 0")
	}
	if result.Error == nil || !strings.Contains(*result.Error, "schema_version") {
		t.Errorf("error must mention schema_version; got %v", result.Error)
	}
}

// Case 5c: missing commit_sha → refuse.
func TestReleaseTag_ReadinessArtifact_MissingCommitSHARefuses(t *testing.T) {
	scratch, wrapper, _, _, _ := setupReleaseTagManifestRepo(t, manifestSpecForReadiness())
	insertReadinessArtifactCommit(t, scratch, manifestSpecForReadiness(), readinessArtifactSpec{
		OmitField: "commit_sha",
	})

	exitCode, result := runReleaseTagFromScratch(t, wrapper)
	if exitCode == 0 {
		t.Fatalf("missing commit_sha must REFUSE; got exit 0")
	}
	if result.Error == nil || !strings.Contains(*result.Error, "commit_sha") {
		t.Errorf("error must mention commit_sha; got %v", result.Error)
	}
}

// Case 5d: missing model_gates → refuse.
func TestReleaseTag_ReadinessArtifact_MissingModelGatesRefuses(t *testing.T) {
	scratch, wrapper, _, _, _ := setupReleaseTagManifestRepo(t, manifestSpecForReadiness())
	insertReadinessArtifactCommit(t, scratch, manifestSpecForReadiness(), readinessArtifactSpec{
		OmitField: "model_gates",
	})

	exitCode, result := runReleaseTagFromScratch(t, wrapper)
	if exitCode == 0 {
		t.Fatalf("missing model_gates must REFUSE; got exit 0")
	}
	if result.Error == nil || !strings.Contains(*result.Error, "model_gates") {
		t.Errorf("error must mention model_gates; got %v", result.Error)
	}
}

// Case 5e: missing a gate key → refuse.
func TestReleaseTag_ReadinessArtifact_MissingGateRefuses(t *testing.T) {
	scratch, wrapper, _, _, _ := setupReleaseTagManifestRepo(t, manifestSpecForReadiness())
	insertReadinessArtifactCommit(t, scratch, manifestSpecForReadiness(), readinessArtifactSpec{
		OmitGate: "G3_docs",
	})

	exitCode, result := runReleaseTagFromScratch(t, wrapper)
	if exitCode == 0 {
		t.Fatalf("missing gate key must REFUSE; got exit 0")
	}
	if result.Error == nil || !strings.Contains(*result.Error, "missing gate G3_docs") {
		t.Errorf("error must mention missing gate G3_docs; got %v", result.Error)
	}
}

// Case 9: unknown verdict value → refuse (schema validation).
func TestReleaseTag_ReadinessArtifact_UnknownVerdictRefuses(t *testing.T) {
	scratch, wrapper, _, _, _ := setupReleaseTagManifestRepo(t, manifestSpecForReadiness())
	insertReadinessArtifactCommit(t, scratch, manifestSpecForReadiness(), readinessArtifactSpec{
		Gates: map[string]string{"G2_significance": "pending"},
	})

	exitCode, result := runReleaseTagFromScratch(t, wrapper)
	if exitCode == 0 {
		t.Fatalf("unknown verdict must REFUSE; got exit 0")
	}
	if result.Error == nil || !strings.Contains(*result.Error, "unknown verdict") {
		t.Errorf("error must mention unknown verdict; got %v", result.Error)
	}
}

// Case 6: commit_sha ≠ HEAD^^ → refuse.
func TestReleaseTag_ReadinessArtifact_CommitSHAMismatchRefuses(t *testing.T) {
	scratch, wrapper, _, _, _ := setupReleaseTagManifestRepo(t, manifestSpecForReadiness())
	// Use a valid 40-hex SHA that is NOT the release-prep SHA.
	insertReadinessArtifactCommit(t, scratch, manifestSpecForReadiness(), readinessArtifactSpec{
		CommitSHA: "0000000000000000000000000000000000000000",
	})

	exitCode, result := runReleaseTagFromScratch(t, wrapper)
	if exitCode == 0 {
		t.Fatalf("commit_sha mismatch must REFUSE; got exit 0")
	}
	if result.Error == nil || !strings.Contains(*result.Error, "commit_sha") {
		t.Errorf("error must mention commit_sha; got %v", result.Error)
	}
	if result.Error == nil || !strings.Contains(*result.Error, "does not match") {
		t.Errorf("error must mention SHA mismatch; got %v", result.Error)
	}
}

// Case 7: any gate = blocked → refuse.
func TestReleaseTag_ReadinessArtifact_GateBlockedRefuses(t *testing.T) {
	scratch, wrapper, _, _, _ := setupReleaseTagManifestRepo(t, manifestSpecForReadiness())
	insertReadinessArtifactCommit(t, scratch, manifestSpecForReadiness(), readinessArtifactSpec{
		Gates: map[string]string{"G3_docs": "blocked"},
	})

	exitCode, result := runReleaseTagFromScratch(t, wrapper)
	if exitCode == 0 {
		t.Fatalf("blocked gate must REFUSE; got exit 0")
	}
	if result.Error == nil || !strings.Contains(*result.Error, "not all ready") {
		t.Errorf("error must mention not all ready; got %v", result.Error)
	}
	if result.Error == nil || !strings.Contains(*result.Error, "blocked") {
		t.Errorf("error must mention blocked gate; got %v", result.Error)
	}
}

// Case 8: any gate = skipped → refuse.
func TestReleaseTag_ReadinessArtifact_GateSkippedRefuses(t *testing.T) {
	scratch, wrapper, _, _, _ := setupReleaseTagManifestRepo(t, manifestSpecForReadiness())
	insertReadinessArtifactCommit(t, scratch, manifestSpecForReadiness(), readinessArtifactSpec{
		Gates: map[string]string{"G5_curated_note": "skipped"},
	})

	exitCode, result := runReleaseTagFromScratch(t, wrapper)
	if exitCode == 0 {
		t.Fatalf("skipped gate must REFUSE; got exit 0")
	}
	if result.Error == nil || !strings.Contains(*result.Error, "not all ready") {
		t.Errorf("error must mention not all ready; got %v", result.Error)
	}
	if result.Error == nil || !strings.Contains(*result.Error, "skipped") {
		t.Errorf("error must mention skipped gate; got %v", result.Error)
	}
}

// Non-single-path artifact commit → refuse (the artifact commit changed more
// than just the readiness artifact path).
func TestReleaseTag_ReadinessArtifact_NonSinglePathArtifactCommitRefuses(t *testing.T) {
	scratch, wrapper, _, _, _ := setupReleaseTagManifestRepo(t, manifestSpecForReadiness())

	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", scratch}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	// Capture release-prep SHA for the artifact commit_sha.
	out, err := exec.Command("git", "-C", scratch, "rev-parse", "HEAD^").Output()
	if err != nil {
		t.Fatalf("git rev-parse HEAD^: %v", err)
	}
	releasePrepSHA := strings.TrimSpace(string(out))

	// Undo manifest, unstage it, then commit artifact + an extra file.
	git("reset", "--soft", "HEAD~1")
	git("reset", "HEAD", "--", ".vh-agent-harness/release-defer-dispositions.json")

	artifactBytes := buildReadinessArtifactBytes(t, readinessArtifactSpec{CommitSHA: releasePrepSHA})
	artifactPath := filepath.Join(scratch, ".vh-agent-harness", "release-readiness-pass.json")
	if err := os.MkdirAll(filepath.Dir(artifactPath), 0o755); err != nil {
		t.Fatalf("mkdir artifact dir: %v", err)
	}
	if err := os.WriteFile(artifactPath, artifactBytes, 0o644); err != nil {
		t.Fatalf("write artifact: %v", err)
	}
	// Also write an extra file so the artifact commit is not single-path.
	extraPath := filepath.Join(scratch, "extra.go")
	if err := os.WriteFile(extraPath, []byte("package scratch\n\nconst ExtraChange = true\n"), 0o644); err != nil {
		t.Fatalf("write extra.go: %v", err)
	}
	git("add", ".vh-agent-harness/release-readiness-pass.json", "extra.go")
	git("commit", "-q", "-m", "artifact + extra (non-single-path)")

	// Re-stamp the manifest so DEFER passes.
	restampManifest(t, scratch, manifestSpecForReadiness())

	exitCode, result := runReleaseTagFromScratch(t, wrapper)
	if exitCode == 0 {
		t.Fatalf("non-single-path artifact commit must REFUSE; got exit 0")
	}
	if result.Error == nil || !strings.Contains(*result.Error, "must change only") {
		t.Errorf("error must mention single-path requirement; got %v", result.Error)
	}
}

// Shallow history (no HEAD^^) → refuse. The readiness ceremony requires at
// least note → artifact → manifest sequencing; a 2-commit repo cannot satisfy it.
func TestReleaseTag_ReadinessArtifact_ShallowHistoryRefuses(t *testing.T) {
	scratch, wrapper, _, _, _ := setupReleaseTagManifestRepo(t, manifestSpecForReadiness())
	// Squash to a 2-commit history: reset to the initial commit (v0.1.0 tag),
	// then commit only the manifest on top. HEAD^ = initial (exists), HEAD^^ =
	// (does not exist). The worktree must stay clean so G0b does not fire
	// before the readiness gate.
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", scratch}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git("reset", "--hard", "v0.1.0")
	// Re-stamp the manifest directly on top of the initial commit. No arc
	// file changes — the worktree stays clean.
	restampManifest(t, scratch, manifestSpecForReadiness())
	// Now HEAD = manifest, HEAD^ = initial (only 2 commits). HEAD^^ does not exist.

	exitCode, result := runReleaseTagFromScratch(t, wrapper)
	if exitCode == 0 {
		t.Fatalf("shallow history must REFUSE; got exit 0")
	}
	if result.Error == nil || !strings.Contains(*result.Error, "HEAD^^ does not exist") {
		t.Errorf("error must mention HEAD^^ does not exist; got %v", result.Error)
	}
}
