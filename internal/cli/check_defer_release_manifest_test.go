package cli

// MANIFEST-AUTHORITY release-mode tests for check-defer-triggers.js.
//
// Release mode reads the committed manifest at
// .vh-agent-harness/release-defer-dispositions.json ONLY (no .local/ access).
// This file pins the schema-v1, handshake, disposition matrix, and freshness
// contract of the manifest-authority path.
//
// The legacy .local/-scan release path has been RETIRED; manifest authority is
// the sole release-authority model. Promoter mode
// (TestCheckDefer_PromoterModeUnchanged in check_defer_release_test.go) is the
// separate commit-time DEFER check and stays unchanged.

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

// manifestOverrideSpec models the override object inside a record.
type manifestOverrideSpec struct {
	ReleaseVersion string
	ApprovedBy     string
	ApprovedAt     string
	Reason         string
}

// manifestRecordSpec models one record. Fields map 1:1 to the schema.
type manifestRecordSpec struct {
	DeferID          string
	ReleaseRelevance string // yes|no|unknown
	Disposition      string // block|disclose|override_required
	MetadataState    string // valid|stale|invalid
	Summary          string
	Reason           string
	SourceRef        string
	StudiedAt        string
	ReviewedAt       string
	Override         *manifestOverrideSpec // nil → null
}

// manifestSpec models the whole manifest. Corruption hooks (Forged*) let
// negative tests inject handshake-invalid values while keeping the rest valid.
type manifestSpec struct {
	ReleaseBaseKind      string // root|tag
	ReleaseBaseValue     string // tag value; "" for root
	ReconciliationScope  string
	ZeroRecordsConfirmed bool
	Records              []manifestRecordSpec

	// Corruption hooks. When non-empty, replace the real handshake value.
	ForgedEvaluatedCommit string
	ForgedEvaluatedTree   string
	ForgedManifestParent  string
	// ForgedSchemaVersion overrides schema_version when non-zero (default 1).
	ForgedSchemaVersion int
}

// buildManifestBytes renders the spec to manifest JSON, filling in the
// handshake SHAs from the current HEAD. Tests pass the resulting bytes to
// commitReleaseManifest.
func buildManifestBytes(t *testing.T, scratch string, spec manifestSpec) []byte {
	t.Helper()
	parentCommit := gitRevParseVerify(t, scratch, "HEAD")
	parentTree := gitRevParseVerify(t, scratch, "HEAD^{tree}")

	ec := parentCommit
	if spec.ForgedEvaluatedCommit != "" {
		ec = spec.ForgedEvaluatedCommit
	}
	et := parentTree
	if spec.ForgedEvaluatedTree != "" {
		et = spec.ForgedEvaluatedTree
	}
	mp := parentCommit
	if spec.ForgedManifestParent != "" {
		mp = spec.ForgedManifestParent
	}
	schemaV := 1
	if spec.ForgedSchemaVersion != 0 {
		schemaV = spec.ForgedSchemaVersion
	}

	rb := map[string]interface{}{"kind": spec.ReleaseBaseKind}
	if spec.ReleaseBaseKind == "root" {
		rb["value"] = nil
	} else {
		rb["value"] = spec.ReleaseBaseValue
	}

	records := []map[string]interface{}{}
	for _, r := range spec.Records {
		rec := map[string]interface{}{
			"defer_id":          r.DeferID,
			"release_relevance": r.ReleaseRelevance,
			"disposition":       r.Disposition,
			"metadata_state":    r.MetadataState,
			"summary":           r.Summary,
			"reason":            r.Reason,
			"source_ref":        r.SourceRef,
			"studied_at":        r.StudiedAt,
			"reviewed_at":       r.ReviewedAt,
			"override":          nil,
		}
		if r.Override != nil {
			rec["override"] = map[string]interface{}{
				"release_version": r.Override.ReleaseVersion,
				"approved_by":     r.Override.ApprovedBy,
				"approved_at":     r.Override.ApprovedAt,
				"reason":          r.Override.Reason,
			}
		}
		records = append(records, rec)
	}

	scope := spec.ReconciliationScope
	if scope == "" {
		scope = "release arc through evaluated_commit"
	}

	obj := map[string]interface{}{
		"schema_version":         schemaV,
		"release_base":           rb,
		"evaluated_commit":       ec,
		"evaluated_tree":         et,
		"manifest_parent_commit": mp,
		"reconciliation": map[string]interface{}{
			"status":                 "confirmed",
			"scope":                  scope,
			"zero_records_confirmed": spec.ZeroRecordsConfirmed,
		},
		"records": records,
	}
	b, err := json.MarshalIndent(obj, "", "  ")
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	return b
}

// gitRevParseVerify runs `git rev-parse --verify --quiet <ref>` and returns
// the trimmed output (fails the test on error).
func gitRevParseVerify(t *testing.T, scratch, ref string) string {
	t.Helper()
	cmd := exec.Command("git", "-C", scratch, "rev-parse", "--verify", "--quiet", ref)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git rev-parse %s: %v", ref, err)
	}
	return strings.TrimSpace(string(out))
}

// commitReleaseManifest writes the manifest bytes to the conventional path
// <scratch>/.vh-agent-harness/release-defer-dispositions.json, stages ONLY the
// manifest (plus extraStageRel if non-empty), and commits. After this call
// HEAD is the manifest commit M and HEAD^ is the evaluated commit P. The
// manifest's evaluated_commit et al. must already encode P's real SHAs (use
// buildManifestBytes to fill them in).
//
// Returns the absolute manifest path.
func commitReleaseManifest(t *testing.T, scratch string, manifestBytes []byte, extraStageRel string) string {
	t.Helper()
	manifestPath := filepath.Join(scratch, ".vh-agent-harness", "release-defer-dispositions.json")
	if err := os.MkdirAll(filepath.Dir(manifestPath), 0o755); err != nil {
		t.Fatalf("mkdir manifest dir: %v", err)
	}
	if err := os.WriteFile(manifestPath, manifestBytes, 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", scratch}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git("add", ".vh-agent-harness/release-defer-dispositions.json")
	if extraStageRel != "" {
		git("add", extraStageRel)
	}
	git("commit", "-q", "-m", "release-defer manifest")
	return manifestPath
}

// manifestAdvisory models one non-fatal advisory entry the evaluator emits
// when an attested manifest field disagrees with a derived authoritative
// value (e.g. a stale release_base.value). Advisories never change the
// classification.
type manifestAdvisory struct {
	Field    string `json:"field"`
	Severity string `json:"severity"`
	Attested string `json:"attested"`
	Derived  string `json:"derived"`
	Note     string `json:"note"`
}

// manifestResult is the parsed JSON envelope the manifest-authority evaluator
// emits. Only the fields asserted by the tests are typed.
type manifestResult struct {
	Mode              string                   `json:"mode"`
	ManifestAuthority bool                     `json:"manifest_authority"`
	ManifestPath      string                   `json:"manifest_path"`
	ManifestSHA       string                   `json:"manifest_sha"`
	ReleaseVersion    *string                  `json:"release_version"`
	ReleaseBase       map[string]interface{}   `json:"release_base"`
	EvaluatedCommit   string                   `json:"evaluated_commit"`
	EvaluatedTree     string                   `json:"evaluated_tree"`
	ManifestParent    string                   `json:"manifest_parent_commit"`
	HeadParent        string                   `json:"head_parent"`
	HeadParentTree    string                   `json:"head_parent_tree"`
	Reconciliation    map[string]interface{}   `json:"reconciliation"`
	Records           []map[string]interface{} `json:"records"`
	Disclosures       []map[string]interface{} `json:"disclosures"`
	AcceptedOverrides []map[string]interface{} `json:"accepted_overrides"`
	Refusals          []map[string]interface{} `json:"refusals"`
	BlockingIDs       []string                 `json:"blocking_ids"`
	DiscloseIDs       []string                 `json:"disclose_ids"`
	EvaluatorErrIDs   []string                 `json:"evaluator_error_ids"`
	Advisories        []manifestAdvisory       `json:"advisories"`
	Classification    string                   `json:"classification"`
	Error             *string                  `json:"error"`
}

// runReleaseEvalManifest runs the evaluator in release (manifest-authority)
// mode. Returns (exitCode, parsedResult, stdout, stderr). cwd is set to the
// scratch repo root derived from the script path so repoRoot() resolves
// correctly. Manifest authority is always-on post-retirement; no env switch
// is required.
func runReleaseEvalManifest(t *testing.T, script string, extraArgs ...string) (int, manifestResult, string, string) {
	t.Helper()
	nodeBin, err := exec.LookPath("node")
	if err != nil {
		t.Fatalf("node not on PATH: %v", err)
	}
	args := []string{script, "--mode=release"}
	args = append(args, extraArgs...)
	cmd := exec.Command(nodeBin, args...)
	cmd.Env = os.Environ()
	cmd.Dir = filepath.Dir(filepath.Dir(filepath.Dir(script))) // <scratch>
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
			t.Fatalf("node spawn error: %v\nstderr: %s", runErr, errb.String())
		}
	}
	var result manifestResult
	stdout := outb.String()
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("manifest-mode output must be valid JSON (exit=%d): %v\nstdout:\n%s\nstderr:\n%s",
			exitCode, err, stdout, errb.String())
	}
	return exitCode, result, stdout, errb.String()
}

// seededNoDiscloseInvalid is the seed-shape record (matches the canonical
// defer-p1-lineage-002-001 disposition: not release-relevant, disclosed with
// invalid metadata). Reused across multiple tests.
func seededNoDiscloseInvalid(id string) manifestRecordSpec {
	return manifestRecordSpec{
		DeferID:          id,
		ReleaseRelevance: "no",
		Disposition:      "disclose",
		MetadataState:    "invalid",
		Summary:          "Conditional future prune-phase coverage follow-up.",
		Reason:           "Not part of the current release; retained for audit with unsupported legacy trigger metadata.",
		SourceRef:        ".local/coordinator/tasks/" + id + ".json",
		StudiedAt:        "2026-07-15T00:00:00Z",
		ReviewedAt:       "2026-07-20T00:00:00Z",
	}
}

// =============================================================================
// Tests 1–2: manifest mode does not touch .local/
// =============================================================================

// TestCheckDefer_Manifest_UsableWithoutLocal — manifest-mode evaluation works
// when .local/ is entirely absent. The committed manifest is the release
// truth; the evaluator performs NO .local/ access. (Matrix case 1.)
func TestCheckDefer_Manifest_UsableWithoutLocal(t *testing.T) {
	scratch, script, _ := setupReleaseEvalRepo(t)
	spec := manifestSpec{
		ReleaseBaseKind:     "tag",
		ReleaseBaseValue:    "v0.1.0",
		ReconciliationScope: "release arc from v0.1.0 through evaluated_commit",
		Records:             []manifestRecordSpec{seededNoDiscloseInvalid("defer-seed-a")},
	}
	manifestBytes := buildManifestBytes(t, scratch, spec)
	commitReleaseManifest(t, scratch, manifestBytes, "")
	// Verify .local/ does not exist in the scratch repo — manifest mode must
	// not require it.
	if _, err := os.Stat(filepath.Join(scratch, ".local")); !os.IsNotExist(err) {
		t.Fatalf(".local/ must not exist for this test; stat err=%v", err)
	}
	exitCode, result, _, _ := runReleaseEvalManifest(t, script)
	if exitCode != 0 {
		t.Fatalf("manifest with .local absent must PASS (exit 0); got %d", exitCode)
	}
	if !result.ManifestAuthority {
		t.Errorf("manifest_authority must be true; got %v", result.ManifestAuthority)
	}
	if result.Classification != "disclose" {
		t.Errorf("single no+disclose record → disclose; got %q", result.Classification)
	}
	if len(result.DiscloseIDs) != 1 || result.DiscloseIDs[0] != "defer-seed-a" {
		t.Errorf("disclose_ids = [defer-seed-a]; got %v", result.DiscloseIDs)
	}
}

// TestCheckDefer_Manifest_MalformedLocalCardIgnored — a malformed .local/
// card MUST NOT affect manifest-mode evaluation. The committed manifest is
// the only input; .local/ is provenance transport only. (Matrix case 2.)
func TestCheckDefer_Manifest_MalformedLocalCardIgnored(t *testing.T) {
	scratch, script, _ := setupReleaseEvalRepo(t)
	// Drop a malformed card into .local/coordinator/tasks/ (the legacy path's
	// default scan dir). Manifest mode must not read it.
	localTasksDir := filepath.Join(scratch, ".local", "coordinator", "tasks")
	if err := os.MkdirAll(localTasksDir, 0o755); err != nil {
		t.Fatalf("mkdir .local tasks: %v", err)
	}
	if err := os.WriteFile(filepath.Join(localTasksDir, "broken.json"), []byte("{not json"), 0o644); err != nil {
		t.Fatalf("write broken card: %v", err)
	}
	spec := manifestSpec{
		ReleaseBaseKind:     "tag",
		ReleaseBaseValue:    "v0.1.0",
		ReconciliationScope: "release arc from v0.1.0 through evaluated_commit",
		Records:             []manifestRecordSpec{seededNoDiscloseInvalid("defer-seed-b")},
	}
	manifestBytes := buildManifestBytes(t, scratch, spec)
	commitReleaseManifest(t, scratch, manifestBytes, "")
	exitCode, result, _, _ := runReleaseEvalManifest(t, script)
	if exitCode != 0 {
		t.Fatalf("manifest mode ignores malformed .local/ card; must PASS (exit 0); got %d", exitCode)
	}
	if result.Classification != "disclose" {
		t.Errorf("manifest eval → disclose; got %q (err=%v)", result.Classification, result.Error)
	}
}

// =============================================================================
// Test 3: missing committed manifest refuses after activation
// =============================================================================

// TestCheckDefer_Manifest_MissingRefusesAfterActivation — in manifest-authority
// mode (always-on post-retirement) with no committed manifest, the evaluator
// MUST refuse. This is the core fix: absent transport can no longer masquerade
// as "clear" in release mode. (Matrix case 3.)
func TestCheckDefer_Manifest_MissingRefusesAfterActivation(t *testing.T) {
	_, script, _ := setupReleaseEvalRepo(t)
	// No manifest written. Manifest authority is active → must refuse.
	exitCode, result, _, _ := runReleaseEvalManifest(t, script)
	if exitCode != 2 {
		t.Fatalf("missing manifest with authority active must REFUSE (exit 2); got %d", exitCode)
	}
	if result.Classification != "evaluator-error" {
		t.Errorf("missing manifest → evaluator-error; got %q", result.Classification)
	}
	if result.Error == nil || !strings.Contains(*result.Error, "manifest missing") {
		t.Errorf("error must mention 'manifest missing'; got %v", result.Error)
	}
}

// =============================================================================
// Tests 4–6: schema-level refusals
// =============================================================================

// TestCheckDefer_Manifest_UnsupportedSchemaRefuses — schema_version=2 is
// unsupported → refuse. (Matrix case 4.)
func TestCheckDefer_Manifest_UnsupportedSchemaRefuses(t *testing.T) {
	scratch, script, _ := setupReleaseEvalRepo(t)
	spec := manifestSpec{
		ReleaseBaseKind:     "tag",
		ReleaseBaseValue:    "v0.1.0",
		ReconciliationScope: "release arc from v0.1.0 through evaluated_commit",
		Records:             []manifestRecordSpec{seededNoDiscloseInvalid("defer-seed")},
		ForgedSchemaVersion: 2,
	}
	manifestBytes := buildManifestBytes(t, scratch, spec)
	commitReleaseManifest(t, scratch, manifestBytes, "")
	exitCode, result, _, _ := runReleaseEvalManifest(t, script)
	if exitCode != 2 {
		t.Fatalf("unsupported schema_version must REFUSE (exit 2); got %d", exitCode)
	}
	if result.Classification != "evaluator-error" {
		t.Errorf("unsupported schema → evaluator-error; got %q", result.Classification)
	}
	if result.Error == nil || !strings.Contains(*result.Error, "schema_version") {
		t.Errorf("error must mention schema_version; got %v", result.Error)
	}
}

// TestCheckDefer_Manifest_DuplicateRecordIDRefuses — two records with the
// same defer_id → refuse. (Matrix case 5.)
func TestCheckDefer_Manifest_DuplicateRecordIDRefuses(t *testing.T) {
	scratch, script, _ := setupReleaseEvalRepo(t)
	spec := manifestSpec{
		ReleaseBaseKind:     "tag",
		ReleaseBaseValue:    "v0.1.0",
		ReconciliationScope: "release arc from v0.1.0 through evaluated_commit",
		Records: []manifestRecordSpec{
			seededNoDiscloseInvalid("defer-dup"),
			seededNoDiscloseInvalid("defer-dup"),
		},
	}
	manifestBytes := buildManifestBytes(t, scratch, spec)
	commitReleaseManifest(t, scratch, manifestBytes, "")
	exitCode, result, _, _ := runReleaseEvalManifest(t, script)
	if exitCode != 2 {
		t.Fatalf("duplicate defer_id must REFUSE (exit 2); got %d", exitCode)
	}
	if result.Classification != "evaluator-error" {
		t.Errorf("duplicate ID → evaluator-error; got %q", result.Classification)
	}
	if result.Error == nil || !strings.Contains(*result.Error, "duplicate") {
		t.Errorf("error must mention duplicate; got %v", result.Error)
	}
}

// TestCheckDefer_Manifest_UnknownRelevanceRefuses — release_relevance=maybe
// is not a valid enum → refuse. (Matrix case 6.)
func TestCheckDefer_Manifest_UnknownRelevanceRefuses(t *testing.T) {
	scratch, script, _ := setupReleaseEvalRepo(t)
	r := seededNoDiscloseInvalid("defer-x")
	r.ReleaseRelevance = "maybe"
	spec := manifestSpec{
		ReleaseBaseKind:     "tag",
		ReleaseBaseValue:    "v0.1.0",
		ReconciliationScope: "release arc from v0.1.0 through evaluated_commit",
		Records:             []manifestRecordSpec{r},
	}
	manifestBytes := buildManifestBytes(t, scratch, spec)
	commitReleaseManifest(t, scratch, manifestBytes, "")
	exitCode, result, _, _ := runReleaseEvalManifest(t, script)
	if exitCode != 2 {
		t.Fatalf("unknown release_relevance must REFUSE (exit 2); got %d", exitCode)
	}
	if result.Classification != "evaluator-error" {
		t.Errorf("unknown relevance → evaluator-error; got %q", result.Classification)
	}
}

// =============================================================================
// Tests 7–11: disposition matrix
// =============================================================================

// TestCheckDefer_Manifest_RelevantBlockRefuses — release_relevance=yes with
// disposition=block is a hard block. (Matrix case 7.)
func TestCheckDefer_Manifest_RelevantBlockRefuses(t *testing.T) {
	scratch, script, _ := setupReleaseEvalRepo(t)
	r := seededNoDiscloseInvalid("defer-block")
	r.ReleaseRelevance = "yes"
	r.Disposition = "block"
	r.MetadataState = "valid"
	spec := manifestSpec{
		ReleaseBaseKind:     "tag",
		ReleaseBaseValue:    "v0.1.0",
		ReconciliationScope: "release arc from v0.1.0 through evaluated_commit",
		Records:             []manifestRecordSpec{r},
	}
	manifestBytes := buildManifestBytes(t, scratch, spec)
	commitReleaseManifest(t, scratch, manifestBytes, "")
	exitCode, result, _, _ := runReleaseEvalManifest(t, script)
	if exitCode != 1 {
		t.Fatalf("yes+block must REFUSE (exit 1, blocker); got %d", exitCode)
	}
	if result.Classification != "blocker" {
		t.Errorf("yes+block → blocker; got %q", result.Classification)
	}
	if len(result.BlockingIDs) != 1 || result.BlockingIDs[0] != "defer-block" {
		t.Errorf("blocking_ids = [defer-block]; got %v", result.BlockingIDs)
	}
}

// TestCheckDefer_Manifest_RelevantValidDiscloseAllows — release_relevance=yes
// with disposition=disclose and metadata_state=valid is allow + disclose.
// (Matrix case 8.)
func TestCheckDefer_Manifest_RelevantValidDiscloseAllows(t *testing.T) {
	scratch, script, _ := setupReleaseEvalRepo(t)
	r := seededNoDiscloseInvalid("defer-disclose")
	r.ReleaseRelevance = "yes"
	r.Disposition = "disclose"
	r.MetadataState = "valid"
	spec := manifestSpec{
		ReleaseBaseKind:     "tag",
		ReleaseBaseValue:    "v0.1.0",
		ReconciliationScope: "release arc from v0.1.0 through evaluated_commit",
		Records:             []manifestRecordSpec{r},
	}
	manifestBytes := buildManifestBytes(t, scratch, spec)
	commitReleaseManifest(t, scratch, manifestBytes, "")
	exitCode, result, stdout, _ := runReleaseEvalManifest(t, script)
	if exitCode != 0 {
		t.Fatalf("yes+disclose+valid must ALLOW (exit 0); got %d", exitCode)
	}
	if result.Classification != "disclose" {
		t.Errorf("yes+disclose+valid → disclose; got %q", result.Classification)
	}
	if len(result.DiscloseIDs) != 1 || result.DiscloseIDs[0] != "defer-disclose" {
		t.Errorf("disclose_ids = [defer-disclose]; got %v", result.DiscloseIDs)
	}
	if !strings.Contains(stdout, "defer-disclose") {
		t.Errorf("disclosed record must appear in output; stdout:\n%s", stdout)
	}
}

// TestCheckDefer_Manifest_RelevantInvalidDiscloseRefuses — release_relevance=
// yes with disposition=disclose but metadata_state=invalid → refuse. The
// disclosed-relevant path REQUIRES valid metadata. (Matrix case 9.)
func TestCheckDefer_Manifest_RelevantInvalidDiscloseRefuses(t *testing.T) {
	scratch, script, _ := setupReleaseEvalRepo(t)
	for _, meta := range []string{"stale", "invalid"} {
		r := seededNoDiscloseInvalid("defer-bad")
		r.ReleaseRelevance = "yes"
		r.Disposition = "disclose"
		r.MetadataState = meta
		spec := manifestSpec{
			ReleaseBaseKind:     "tag",
			ReleaseBaseValue:    "v0.1.0",
			ReconciliationScope: "release arc from v0.1.0 through evaluated_commit",
			Records:             []manifestRecordSpec{r},
		}
		// Reset HEAD between iterations by recreating the repo.
		scratch, script, _ = setupReleaseEvalRepo(t)
		manifestBytes := buildManifestBytes(t, scratch, spec)
		commitReleaseManifest(t, scratch, manifestBytes, "")
		exitCode, result, _, _ := runReleaseEvalManifest(t, script)
		if exitCode != 1 {
			t.Fatalf("yes+disclose+%s must REFUSE (exit 1); got %d", meta, exitCode)
		}
		if result.Classification != "blocker" {
			t.Errorf("yes+disclose+%s → blocker; got %q", meta, result.Classification)
		}
	}
}

// TestCheckDefer_Manifest_NoDiscloseInvalidAllows — release_relevance=no with
// disposition=disclose and metadata_state=invalid is allow + disclose. This
// is the canonical seed shape (defer-p1-lineage-002-001). (Matrix case 10.)
func TestCheckDefer_Manifest_NoDiscloseInvalidAllows(t *testing.T) {
	scratch, script, _ := setupReleaseEvalRepo(t)
	spec := manifestSpec{
		ReleaseBaseKind:     "tag",
		ReleaseBaseValue:    "v0.1.0",
		ReconciliationScope: "release arc from v0.1.0 through evaluated_commit",
		Records:             []manifestRecordSpec{seededNoDiscloseInvalid("defer-p1-lineage-002-001")},
	}
	manifestBytes := buildManifestBytes(t, scratch, spec)
	commitReleaseManifest(t, scratch, manifestBytes, "")
	exitCode, result, stdout, _ := runReleaseEvalManifest(t, script)
	if exitCode != 0 {
		t.Fatalf("no+disclose+invalid must ALLOW (exit 0); got %d", exitCode)
	}
	if result.Classification != "disclose" {
		t.Errorf("no+disclose+invalid → disclose; got %q", result.Classification)
	}
	if len(result.DiscloseIDs) != 1 || result.DiscloseIDs[0] != "defer-p1-lineage-002-001" {
		t.Errorf("disclose_ids = [defer-p1-lineage-002-001]; got %v", result.DiscloseIDs)
	}
	if !strings.Contains(stdout, "defer-p1-lineage-002-001") {
		t.Errorf("seed record must appear in output; stdout:\n%s", stdout)
	}
}

// TestCheckDefer_Manifest_NoBlockPolicyErrorRefuses — release_relevance=no
// with disposition=block is a policy error (contradictory). (Matrix case 11.)
func TestCheckDefer_Manifest_NoBlockPolicyErrorRefuses(t *testing.T) {
	scratch, script, _ := setupReleaseEvalRepo(t)
	r := seededNoDiscloseInvalid("defer-policy")
	r.ReleaseRelevance = "no"
	r.Disposition = "block"
	spec := manifestSpec{
		ReleaseBaseKind:     "tag",
		ReleaseBaseValue:    "v0.1.0",
		ReconciliationScope: "release arc from v0.1.0 through evaluated_commit",
		Records:             []manifestRecordSpec{r},
	}
	manifestBytes := buildManifestBytes(t, scratch, spec)
	commitReleaseManifest(t, scratch, manifestBytes, "")
	exitCode, result, _, _ := runReleaseEvalManifest(t, script)
	if exitCode != 1 {
		t.Fatalf("no+block policy error must REFUSE (exit 1); got %d", exitCode)
	}
	if result.Classification != "blocker" {
		t.Errorf("no+block → blocker (policy error); got %q", result.Classification)
	}
	if len(result.Refusals) != 1 {
		t.Errorf("refusals must have 1 entry; got %v", result.Refusals)
	}
}

// =============================================================================
// Tests 12–13: empty records + zero_records_confirmed rule
// =============================================================================

// TestCheckDefer_Manifest_EmptyRecordsNoConfirmationRefuses — empty records
// array with zero_records_confirmed=false is an unattested empty array →
// refuse. (Matrix case 12.)
func TestCheckDefer_Manifest_EmptyRecordsNoConfirmationRefuses(t *testing.T) {
	scratch, script, _ := setupReleaseEvalRepo(t)
	spec := manifestSpec{
		ReleaseBaseKind:      "tag",
		ReleaseBaseValue:     "v0.1.0",
		ReconciliationScope:  "release arc from v0.1.0 through evaluated_commit",
		ZeroRecordsConfirmed: false,
		Records:              []manifestRecordSpec{},
	}
	manifestBytes := buildManifestBytes(t, scratch, spec)
	commitReleaseManifest(t, scratch, manifestBytes, "")
	exitCode, result, _, _ := runReleaseEvalManifest(t, script)
	if exitCode != 2 {
		t.Fatalf("empty records without confirmation must REFUSE (exit 2); got %d", exitCode)
	}
	if result.Classification != "evaluator-error" {
		t.Errorf("empty records no confirm → evaluator-error; got %q", result.Classification)
	}
}

// TestCheckDefer_Manifest_EmptyRecordsConfirmedAllows — empty records array
// with zero_records_confirmed=true is an explicit attestation → allow (clear).
// (Matrix case 13.)
func TestCheckDefer_Manifest_EmptyRecordsConfirmedAllows(t *testing.T) {
	scratch, script, _ := setupReleaseEvalRepo(t)
	spec := manifestSpec{
		ReleaseBaseKind:      "tag",
		ReleaseBaseValue:     "v0.1.0",
		ReconciliationScope:  "release arc from v0.1.0 through evaluated_commit",
		ZeroRecordsConfirmed: true,
		Records:              []manifestRecordSpec{},
	}
	manifestBytes := buildManifestBytes(t, scratch, spec)
	commitReleaseManifest(t, scratch, manifestBytes, "")
	exitCode, result, _, _ := runReleaseEvalManifest(t, script)
	if exitCode != 0 {
		t.Fatalf("empty records with confirmation must ALLOW (exit 0); got %d", exitCode)
	}
	if result.Classification != "clear" {
		t.Errorf("empty records confirmed → clear; got %q", result.Classification)
	}
}

// =============================================================================
// Tests 14–16: release_base
// =============================================================================

// TestCheckDefer_Manifest_RootBaseEvaluatesWholeHistory — release_base.kind=
// root means first release; whole history is in scope; the manifest attests
// relevance for that whole-history arc. No prior-tag check. (Matrix case 14.)
func TestCheckDefer_Manifest_RootBaseEvaluatesWholeHistory(t *testing.T) {
	scratch, script, _ := setupReleaseEvalRepo(t)
	spec := manifestSpec{
		ReleaseBaseKind:     "root",
		ReconciliationScope: "whole repository history through evaluated_commit",
		Records:             []manifestRecordSpec{seededNoDiscloseInvalid("defer-seed")},
	}
	manifestBytes := buildManifestBytes(t, scratch, spec)
	commitReleaseManifest(t, scratch, manifestBytes, "")
	exitCode, result, _, _ := runReleaseEvalManifest(t, script)
	if exitCode != 0 {
		t.Fatalf("root base must ALLOW (exit 0); got %d (err=%v)", exitCode, result.Error)
	}
	if result.Classification != "disclose" {
		t.Errorf("root base → disclose; got %q", result.Classification)
	}
	if rb := result.ReleaseBase; rb == nil || rb["kind"] != "root" {
		t.Errorf("release_base.kind should echo root; got %v", rb)
	}
}

// TestCheckDefer_Manifest_RootBaseIgnoresHeadFallback — manifest mode never
// resolves --since via HEAD~32 fallback. A repo with NO prior tag and a
// root-base manifest evaluates cleanly. The legacy HEAD~32 fail-open path is
// eliminated in manifest mode. (Matrix case 15.)
func TestCheckDefer_Manifest_RootBaseIgnoresHeadFallback(t *testing.T) {
	scratch, script, _ := setupReleaseEvalRepo(t)
	// Delete the prior tag to simulate first-release state.
	cmd := exec.Command("git", "-C", scratch, "tag", "-d", "v0.1.0")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git tag -d v0.1.0: %v\n%s", err, out)
	}
	spec := manifestSpec{
		ReleaseBaseKind:     "root",
		ReconciliationScope: "whole repository history through evaluated_commit",
		Records:             []manifestRecordSpec{seededNoDiscloseInvalid("defer-seed")},
	}
	manifestBytes := buildManifestBytes(t, scratch, spec)
	commitReleaseManifest(t, scratch, manifestBytes, "")
	exitCode, result, _, _ := runReleaseEvalManifest(t, script)
	if exitCode != 0 {
		t.Fatalf("root base with no prior tag must ALLOW (exit 0); got %d (err=%v)", exitCode, result.Error)
	}
	if result.Classification == "evaluator-error" {
		t.Errorf("manifest mode must not fall through to HEAD~32 fail-open; got evaluator-error: %v", result.Error)
	}
}

// TestCheckDefer_Manifest_StaleBaseDerivesAuthoritatively — release_base.kind=
// tag with an attested value that does NOT match the discovered prior tag is
// self-healed by DERIVE-ON-READ: the evaluator derives the authoritative prior
// tag from git and uses it; the attested value is demoted to a non-fatal
// advisory. The release is NOT blocked (PASS / disclose) and the manifest is
// NOT written. (Replaces the prior "wrong prior tag → refuse" matrix case now
// that release_base.value is derived, not operator-attested-authoritative.)
func TestCheckDefer_Manifest_StaleBaseDerivesAuthoritatively(t *testing.T) {
	scratch, script, _ := setupReleaseEvalRepo(t)
	spec := manifestSpec{
		ReleaseBaseKind:     "tag",
		ReleaseBaseValue:    "v9.9.9", // stale/wrong; real prior tag is v0.1.0
		ReconciliationScope: "release arc from v9.9.9 through evaluated_commit",
		Records:             []manifestRecordSpec{seededNoDiscloseInvalid("defer-seed")},
	}
	manifestBytes := buildManifestBytes(t, scratch, spec)
	commitReleaseManifest(t, scratch, manifestBytes, "")
	exitCode, result, _, _ := runReleaseEvalManifest(t, script)
	if exitCode != 0 {
		t.Fatalf("stale attested release_base must PASS via derive-on-read (exit 0); got %d (err=%v)", exitCode, result.Error)
	}
	if result.Classification == "evaluator-error" || result.Classification == "blocker" {
		t.Fatalf("stale attested release_base must not be evaluator-error/blocker; got %q (err=%v)", result.Classification, result.Error)
	}
	// The envelope echoes the DERIVED authoritative value, not the attested.
	rb := result.ReleaseBase
	if rb == nil || rb["kind"] != "tag" || rb["value"] != "v0.1.0" {
		t.Errorf("envelope release_base must echo derived {tag, v0.1.0}; got %v", rb)
	}
	// A non-fatal advisory must be recorded naming the stale attested value.
	if len(result.Advisories) != 1 {
		t.Fatalf("exactly one advisory expected for stale release_base.value; got %d (%+v)", len(result.Advisories), result.Advisories)
	}
	a := result.Advisories[0]
	if a.Field != "release_base.value" || a.Attested != "v9.9.9" || a.Derived != "v0.1.0" || a.Severity != "advisory" {
		t.Errorf("advisory mismatch: %+v", a)
	}
}

// TestCheckDefer_Manifest_TagBaseWithNoPriorTagFailsClosed — kind=tag but NO
// tag is reachable from HEAD (after excluding --release-version) cannot derive
// a release base. That is a genuine malformed-manifest state (a tag release
// with no prior tag), NOT a stale value, so the evaluator fails closed
// (evaluator-error, exit 2). This is the ONLY remaining fail-closed
// release_base condition now that the value is derived.
func TestCheckDefer_Manifest_TagBaseWithNoPriorTagFailsClosed(t *testing.T) {
	scratch, script, _ := setupReleaseEvalRepo(t)
	// Delete the prior tag so none is reachable from HEAD.
	cmd := exec.Command("git", "-C", scratch, "tag", "-d", "v0.1.0")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git tag -d v0.1.0: %v\n%s", err, out)
	}
	spec := manifestSpec{
		ReleaseBaseKind:     "tag",
		ReleaseBaseValue:    "v0.0.99", // irrelevant; no prior tag exists at all
		ReconciliationScope: "release arc through evaluated_commit",
		Records:             []manifestRecordSpec{seededNoDiscloseInvalid("defer-seed")},
	}
	manifestBytes := buildManifestBytes(t, scratch, spec)
	commitReleaseManifest(t, scratch, manifestBytes, "")
	exitCode, result, _, _ := runReleaseEvalManifest(t, script)
	if exitCode != 2 {
		t.Fatalf("kind=tag with no reachable prior tag must FAIL CLOSED (exit 2); got %d", exitCode)
	}
	if result.Classification != "evaluator-error" {
		t.Errorf("no-prior-tag kind=tag → evaluator-error; got %q", result.Classification)
	}
	if result.Error == nil || !strings.Contains(*result.Error, "no prior tag reachable") {
		t.Errorf("error must mention no prior tag reachable; got %v", result.Error)
	}
}

// TestCheckDefer_Manifest_ReleaseBaseReachableOnlyExcludesUnreachableTags —
// pins the BRANCHED-history reachability contract that the linear CI recheck
// test below does NOT exercise. Scenario: a maintenance-branch release
// (v1.0.2, declaring release_base.value=v1.0.0) is cut from a fork point that
// is an ANCESTOR of an unrelated higher mainline tag (v1.1.0 on main). When
// the evaluator is invoked at v1.0.2 with --release-version v1.0.2, it MUST
// select v1.0.0 as the prior tag (the highest version tag REACHABLE from HEAD)
// and NOT v1.1.0 (unreachable from the v1.0.2 commit). The prior
// `git tag --list --sort=-v:refname` lookup found the highest tag in the ENTIRE
// repo, so it returned the unreachable v1.1.0, emitted release_base mismatch,
// and CI's set -e blocked GoReleaser publication.
//
// Before the reachability fix: gitLatestTag("v1.0.2") scanned all tags and
// returned v1.1.0 → mismatch with v1.0.0 → exit 2. After the reachability fix:
// gitLatestTag("v1.0.2") restricts the lookup to `git tag --merged HEAD` (only
// tags reachable from HEAD), so v1.1.0 is filtered out and v1.0.0 is returned.
// Since release_base.value became DERIVE-ON-READ authoritative (a stale
// attested value is now a non-fatal advisory, not exit 2), this test's
// load-bearing claim is that the DERIVED tag is the reachable v1.0.0 (not the
// unreachable v1.1.0) and the manifest's attested v1.0.0 agrees with it → exit 0.
func TestCheckDefer_Manifest_ReleaseBaseReachableOnlyExcludesUnreachableTags(t *testing.T) {
	scratch, script, _ := setupReleaseEvalRepo(t)
	// setupReleaseEvalRepo leaves HEAD two commits past v0.1.0. To build a
	// clean branched scenario we rebuild the tag graph from scratch:
	//
	//   A:v1.0.0 ── B:v1.1.0   (main, unreachable from the maintenance branch)
	//      └─ C:v1.0.2          (maintenance release; release_base.value=v1.0.0)
	//
	// We reuse the existing commits but re-tag so the topology matches: A is
	// the initial commit, B is the post-v0.1.0 commit on the implicit main
	// line, and C is a new commit branched off A.
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", scratch}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	// Clear existing tags and re-create the branched topology.
	git("tag", "-d", "v0.1.0")
	// A = HEAD~ = the initial commit; tag it v1.0.0.
	initialSha := gitRevParseVerify(t, scratch, "HEAD~")
	git("tag", "v1.0.0", initialSha)
	// B = HEAD (the "changes for release" commit on the implicit main line);
	// tag it v1.1.0 — this is the UNREACHABLE higher mainline tag.
	headSha := gitRevParseVerify(t, scratch, "HEAD")
	git("tag", "v1.1.0", headSha)
	// Create the maintenance branch off A (v1.0.0), commit C on it, tag v1.0.2.
	git("branch", "maint", initialSha)
	// Detach onto maint tip (= A) so subsequent commits land on the branch
	// without disturbing main (B). Then add C and tag v1.0.2.
	if out, err := exec.Command("git", "-C", scratch, "checkout", "-q", "maint").CombinedOutput(); err != nil {
		t.Fatalf("git checkout maint: %v\n%s", err, out)
	}
	cRel := "maint-only-change.txt"
	if err := os.WriteFile(filepath.Join(scratch, cRel), []byte("maintenance fix\n"), 0o644); err != nil {
		t.Fatalf("write %s: %v", cRel, err)
	}
	git("add", cRel)
	git("commit", "-q", "-m", "maintenance fix on v1.0.x branch")
	// Tag v1.0.2 at C. release_base will declare v1.0.0 (reachable ancestor).
	git("tag", "v1.0.2")

	// Build a manifest whose release_base.value=v1.0.0 (the correct reachable
	// prior tag). The handshake SHAs must reflect HEAD (= C, the v1.0.2 commit)
	// as evaluated_commit/manifest_parent and HEAD^{tree} as evaluated_tree —
	// buildManifestBytes fills these from the current HEAD. Then commit ONLY the
	// manifest so HEAD^..HEAD == manifest path (handshake invariant).
	spec := manifestSpec{
		ReleaseBaseKind:     "tag",
		ReleaseBaseValue:    "v1.0.0",
		ReconciliationScope: "release arc from v1.0.0 through evaluated_commit",
		Records:             []manifestRecordSpec{seededNoDiscloseInvalid("defer-seed-reach")},
	}
	manifestBytes := buildManifestBytes(t, scratch, spec)
	commitReleaseManifest(t, scratch, manifestBytes, "")

	// Sanity: v1.1.0 is unreachable from HEAD (the v1.0.2 commit), and v1.0.0
	// is reachable. This is the reachability guarantee the fix relies on.
	mergedOut, err := exec.Command("git", "-C", scratch, "tag", "--merged", "HEAD").CombinedOutput()
	if err != nil {
		t.Fatalf("git tag --merged HEAD: %v\n%s", err, mergedOut)
	}
	mergedTags := strings.TrimSpace(string(mergedOut))
	if strings.Contains(mergedTags, "v1.1.0") {
		t.Fatalf("precondition: v1.1.0 must NOT be reachable from HEAD; got --merged HEAD = %q", mergedTags)
	}
	if !strings.Contains(mergedTags, "v1.0.0") {
		t.Fatalf("precondition: v1.0.0 must be reachable from HEAD; got --merged HEAD = %q", mergedTags)
	}

	// CI invocation at v1.0.2: --release-version v1.0.2 forwards the just-cut
	// tag so it is excluded from the lookup. The evaluator MUST resolve the
	// prior tag to v1.0.0 (reachable), NOT v1.1.0 (unreachable) → exit 0.
	exitCode, result, _, _ := runReleaseEvalManifest(t, script, "--release-version", "v1.0.2")
	if exitCode != 0 {
		t.Fatalf("branched-history CI recheck must PASS (exit 0) — release_base should resolve to reachable v1.0.0, not unreachable v1.1.0; got exit=%d (err=%v)", exitCode, result.Error)
	}
	if result.Classification == "evaluator-error" {
		t.Errorf("branched-history CI recheck must not be evaluator-error; got %v", result.Error)
	}
}

// TestCheckDefer_Manifest_ReleaseBaseExcludesReleaseVersionInCIRecheck —
// simulates the CI post-tag recheck scenario. The wrapper already cut v0.2.0
// at HEAD; CI invokes the evaluator with --release-version v0.2.0. The
// manifest's release_base.value names the PRIOR tag (v0.1.0). The evaluator
// must identify v0.1.0 as the release base (NOT v0.2.0), otherwise every
// normal manifest-mode release fails CI publication.
//
// Before the fix: gitLatestTag() returned v0.2.0 (the just-cut tag) → mismatch
// with v0.1.0 → exit 2. After the fix: gitLatestTag("v0.2.0") excludes the
// release version and returns v0.1.0 → match → exit 0.
func TestCheckDefer_Manifest_ReleaseBaseExcludesReleaseVersionInCIRecheck(t *testing.T) {
	scratch, script, _ := setupReleaseEvalRepo(t)
	spec := manifestSpec{
		ReleaseBaseKind:     "tag",
		ReleaseBaseValue:    "v0.1.0", // the PRIOR tag, not the new release
		ReconciliationScope: "release arc from v0.1.0 through evaluated_commit",
		Records:             []manifestRecordSpec{seededNoDiscloseInvalid("defer-seed")},
	}
	manifestBytes := buildManifestBytes(t, scratch, spec)
	commitReleaseManifest(t, scratch, manifestBytes, "")
	// Simulate the post-tag CI state: the wrapper already cut v0.2.0 at HEAD.
	tagCmd := exec.Command("git", "-C", scratch, "tag", "v0.2.0")
	if out, err := tagCmd.CombinedOutput(); err != nil {
		t.Fatalf("git tag v0.2.0: %v\n%s", err, out)
	}
	// CI invocation: --release-version v0.2.0 (the just-cut tag).
	exitCode, result, _, _ := runReleaseEvalManifest(t, script, "--release-version", "v0.2.0")
	if exitCode != 0 {
		t.Fatalf("post-tag CI recheck must PASS (exit 0) — release_base should resolve to v0.1.0; got %d (err=%v)", exitCode, result.Error)
	}
	if result.Classification == "evaluator-error" {
		t.Errorf("post-tag CI recheck must not be evaluator-error; got %v", result.Error)
	}
}

// =============================================================================
// Tests 17–20: freshness handshake (sacred — do not weaken)
// =============================================================================

const fakeShaA = "0000000000000000000000000000000000000001"
const fakeShaB = "0000000000000000000000000000000000000002"

// TestCheckDefer_Manifest_EvaluatedCommitMismatchRefuses — evaluated_commit
// in the manifest != HEAD^ → refuse. (Matrix case 17.)
func TestCheckDefer_Manifest_EvaluatedCommitMismatchRefuses(t *testing.T) {
	scratch, script, _ := setupReleaseEvalRepo(t)
	spec := manifestSpec{
		ReleaseBaseKind:       "tag",
		ReleaseBaseValue:      "v0.1.0",
		ReconciliationScope:   "release arc from v0.1.0 through evaluated_commit",
		Records:               []manifestRecordSpec{seededNoDiscloseInvalid("defer-seed")},
		ForgedEvaluatedCommit: fakeShaA,
	}
	manifestBytes := buildManifestBytes(t, scratch, spec)
	commitReleaseManifest(t, scratch, manifestBytes, "")
	exitCode, result, _, _ := runReleaseEvalManifest(t, script)
	if exitCode != 2 {
		t.Fatalf("evaluated_commit != HEAD^ must REFUSE (exit 2); got %d", exitCode)
	}
	if result.Error == nil || !strings.Contains(*result.Error, "evaluated_commit") {
		t.Errorf("error must mention evaluated_commit mismatch; got %v", result.Error)
	}
}

// TestCheckDefer_Manifest_ManifestParentMismatchRefuses — manifest_parent_
// commit != HEAD^ → refuse. (Matrix case 18.)
func TestCheckDefer_Manifest_ManifestParentMismatchRefuses(t *testing.T) {
	scratch, script, _ := setupReleaseEvalRepo(t)
	spec := manifestSpec{
		ReleaseBaseKind:      "tag",
		ReleaseBaseValue:     "v0.1.0",
		ReconciliationScope:  "release arc from v0.1.0 through evaluated_commit",
		Records:              []manifestRecordSpec{seededNoDiscloseInvalid("defer-seed")},
		ForgedManifestParent: fakeShaA,
	}
	manifestBytes := buildManifestBytes(t, scratch, spec)
	commitReleaseManifest(t, scratch, manifestBytes, "")
	exitCode, result, _, _ := runReleaseEvalManifest(t, script)
	if exitCode != 2 {
		t.Fatalf("manifest_parent_commit != HEAD^ must REFUSE (exit 2); got %d", exitCode)
	}
	if result.Error == nil || !strings.Contains(*result.Error, "manifest_parent_commit") {
		t.Errorf("error must mention manifest_parent_commit mismatch; got %v", result.Error)
	}
}

// TestCheckDefer_Manifest_EvaluatedTreeMismatchRefuses — evaluated_tree !=
// tree(HEAD^) → refuse. (Matrix case 19.)
func TestCheckDefer_Manifest_EvaluatedTreeMismatchRefuses(t *testing.T) {
	scratch, script, _ := setupReleaseEvalRepo(t)
	spec := manifestSpec{
		ReleaseBaseKind:     "tag",
		ReleaseBaseValue:    "v0.1.0",
		ReconciliationScope: "release arc from v0.1.0 through evaluated_commit",
		Records:             []manifestRecordSpec{seededNoDiscloseInvalid("defer-seed")},
		ForgedEvaluatedTree: fakeShaB,
	}
	manifestBytes := buildManifestBytes(t, scratch, spec)
	commitReleaseManifest(t, scratch, manifestBytes, "")
	exitCode, result, _, _ := runReleaseEvalManifest(t, script)
	if exitCode != 2 {
		t.Fatalf("evaluated_tree != HEAD^{tree} must REFUSE (exit 2); got %d", exitCode)
	}
	if result.Error == nil || !strings.Contains(*result.Error, "evaluated_tree") {
		t.Errorf("error must mention evaluated_tree mismatch; got %v", result.Error)
	}
}

// TestCheckDefer_Manifest_DiffNotManifestOnlyRefuses — when HEAD^..HEAD
// changes any file OTHER than the manifest itself, the handshake refuses.
// This prevents weakening the manifest after its claimed evaluation.
// (Matrix case 20.)
func TestCheckDefer_Manifest_DiffNotManifestOnlyRefuses(t *testing.T) {
	scratch, script, _ := setupReleaseEvalRepo(t)
	// Write an extra file alongside the manifest so the manifest commit also
	// contains a non-manifest change.
	extraRel := "extra-sneak.txt"
	extraAbs := filepath.Join(scratch, extraRel)
	if err := os.WriteFile(extraAbs, []byte("sneaky\n"), 0o644); err != nil {
		t.Fatalf("write extra: %v", err)
	}
	spec := manifestSpec{
		ReleaseBaseKind:     "tag",
		ReleaseBaseValue:    "v0.1.0",
		ReconciliationScope: "release arc from v0.1.0 through evaluated_commit",
		Records:             []manifestRecordSpec{seededNoDiscloseInvalid("defer-seed")},
	}
	manifestBytes := buildManifestBytes(t, scratch, spec)
	commitReleaseManifest(t, scratch, manifestBytes, extraRel)
	exitCode, result, _, _ := runReleaseEvalManifest(t, script)
	if exitCode != 2 {
		t.Fatalf("non-manifest diff must REFUSE (exit 2); got %d", exitCode)
	}
	if result.Error == nil || !strings.Contains(*result.Error, "must change only the manifest") {
		t.Errorf("error must mention manifest-only diff; got %v", result.Error)
	}
}

// =============================================================================
// Phase 1 override binding (evaluator side; full wrapper ceremony is Phase 2)
// =============================================================================

// TestCheckDefer_Manifest_OverrideRequiredNoVersionRefuses — an override_
// required record without --release-version cannot be validated → refuse.
func TestCheckDefer_Manifest_OverrideRequiredNoVersionRefuses(t *testing.T) {
	scratch, script, _ := setupReleaseEvalRepo(t)
	r := seededNoDiscloseInvalid("defer-ov")
	r.ReleaseRelevance = "yes"
	r.Disposition = "override_required"
	r.MetadataState = "valid"
	r.Override = &manifestOverrideSpec{
		ReleaseVersion: "v0.13.0",
		ApprovedBy:     "operator",
		ApprovedAt:     "2026-07-20T00:00:00Z",
		Reason:         "ship it",
	}
	spec := manifestSpec{
		ReleaseBaseKind:     "tag",
		ReleaseBaseValue:    "v0.1.0",
		ReconciliationScope: "release arc from v0.1.0 through evaluated_commit",
		Records:             []manifestRecordSpec{r},
	}
	manifestBytes := buildManifestBytes(t, scratch, spec)
	commitReleaseManifest(t, scratch, manifestBytes, "")
	// No --release-version passed.
	exitCode, result, _, _ := runReleaseEvalManifest(t, script)
	if exitCode != 1 {
		t.Fatalf("override_required without --release-version must REFUSE (exit 1); got %d", exitCode)
	}
	if result.Classification != "blocker" {
		t.Errorf("override_required no version → blocker; got %q", result.Classification)
	}
}

// TestCheckDefer_Manifest_OverrideRequiredAcceptedWithoutConfirmationInCIMode —
// the evaluator, invoked WITHOUT --override-confirmed-version (simulating the
// CI defense-in-depth recheck), ACCEPTS a well-formed committed override
// object whose release_version matches --release-version. CI verifies Layer A
// (object validity + version match) from the committed manifest alone; it
// does not re-enforce Layer B (operator live intent) because the wrapper
// already enforced Layer B at tag time and the committed override IS the
// audit trail.
func TestCheckDefer_Manifest_OverrideRequiredAcceptedWithoutConfirmationInCIMode(t *testing.T) {
	scratch, script, _ := setupReleaseEvalRepo(t)
	r := seededNoDiscloseInvalid("defer-ov-ci")
	r.ReleaseRelevance = "yes"
	r.Disposition = "override_required"
	r.MetadataState = "valid"
	r.Override = &manifestOverrideSpec{
		ReleaseVersion: "v0.13.0",
		ApprovedBy:     "operator-alice",
		ApprovedAt:     "2026-07-20T00:00:00Z",
		Reason:         "Acceptable residual risk for v0.13.0.",
	}
	spec := manifestSpec{
		ReleaseBaseKind:     "tag",
		ReleaseBaseValue:    "v0.1.0",
		ReconciliationScope: "release arc from v0.1.0 through evaluated_commit",
		Records:             []manifestRecordSpec{r},
	}
	manifestBytes := buildManifestBytes(t, scratch, spec)
	commitReleaseManifest(t, scratch, manifestBytes, "")
	// CI mode: --release-version matches, but NO --override-confirmed-version.
	exitCode, result, stdout, _ := runReleaseEvalManifest(t, script, "--release-version", "v0.13.0")
	if exitCode != 0 {
		t.Fatalf("CI mode (no --override-confirmed-version) must ACCEPT well-formed override (exit 0); got %d (err=%v)", exitCode, result.Error)
	}
	if result.Classification != "disclose" {
		t.Errorf("CI mode accepted override → disclose; got %q (err=%v)", result.Classification, result.Error)
	}
	if len(result.AcceptedOverrides) != 1 {
		t.Errorf("accepted_overrides must have 1 entry; got %v", result.AcceptedOverrides)
	} else {
		ao := result.AcceptedOverrides[0]
		if ao["defer_id"] != "defer-ov-ci" {
			t.Errorf("accepted_override.defer_id = defer-ov-ci; got %v", ao["defer_id"])
		}
		if ao["approved_by"] != "operator-alice" {
			t.Errorf("accepted_override.approved_by must be disclosed; got %v", ao["approved_by"])
		}
	}
	if !strings.Contains(stdout, "operator-alice") || !strings.Contains(stdout, "v0.13.0") {
		t.Errorf("override must be disclosed in output (approver + version); stdout:\n%s", stdout)
	}
}

// TestCheckDefer_Manifest_OverrideRequiredMismatchedConfirmationRefuses — when
// --override-confirmed-version IS supplied (wrapper mode) but does NOT match
// --release-version, the evaluator REFUSES. This is the wrapper-side Layer B
// enforcement: the operator's live intent confirmation must match the release
// being tagged. The wrapper path supplies this flag only after the ceremony
// (--override-release-version + --override-manifest-sha) succeeds, so a
// mismatch indicates the operator confirmed a different version than the one
// being tagged — refuse.
func TestCheckDefer_Manifest_OverrideRequiredMismatchedConfirmationRefuses(t *testing.T) {
	scratch, script, _ := setupReleaseEvalRepo(t)
	r := seededNoDiscloseInvalid("defer-ov-mismatch")
	r.ReleaseRelevance = "yes"
	r.Disposition = "override_required"
	r.MetadataState = "valid"
	r.Override = &manifestOverrideSpec{
		ReleaseVersion: "v0.13.0",
		ApprovedBy:     "operator",
		ApprovedAt:     "2026-07-20T00:00:00Z",
		Reason:         "ship it",
	}
	spec := manifestSpec{
		ReleaseBaseKind:     "tag",
		ReleaseBaseValue:    "v0.1.0",
		ReconciliationScope: "release arc from v0.1.0 through evaluated_commit",
		Records:             []manifestRecordSpec{r},
	}
	manifestBytes := buildManifestBytes(t, scratch, spec)
	commitReleaseManifest(t, scratch, manifestBytes, "")
	// Wrapper mode: --override-confirmed-version supplied but MISMATCHED.
	exitCode, result, _, _ := runReleaseEvalManifest(t, script,
		"--release-version", "v0.13.0",
		"--override-confirmed-version", "v0.14.0")
	if exitCode != 1 {
		t.Fatalf("mismatched --override-confirmed-version must REFUSE (exit 1); got %d", exitCode)
	}
	if result.Classification != "blocker" {
		t.Errorf("mismatched confirmation → blocker; got %q", result.Classification)
	}
	if result.Error == nil || !strings.Contains(*result.Error, "override-confirmed-version") {
		t.Errorf("error must mention override-confirmed-version; got %v", result.Error)
	}
}

// TestCheckDefer_Manifest_OverrideVersionMismatchRefuses — the override's
// release_version != the --release-version flag → refuse. (Evaluator half of
// matrix case 22; the wrapper adds blob-SHA confirmation in Phase 2.)
func TestCheckDefer_Manifest_OverrideVersionMismatchRefuses(t *testing.T) {
	scratch, script, _ := setupReleaseEvalRepo(t)
	r := seededNoDiscloseInvalid("defer-ov")
	r.ReleaseRelevance = "yes"
	r.Disposition = "override_required"
	r.MetadataState = "valid"
	r.Override = &manifestOverrideSpec{
		ReleaseVersion: "v0.13.0",
		ApprovedBy:     "operator",
		ApprovedAt:     "2026-07-20T00:00:00Z",
		Reason:         "ship it",
	}
	spec := manifestSpec{
		ReleaseBaseKind:     "tag",
		ReleaseBaseValue:    "v0.1.0",
		ReconciliationScope: "release arc from v0.1.0 through evaluated_commit",
		Records:             []manifestRecordSpec{r},
	}
	manifestBytes := buildManifestBytes(t, scratch, spec)
	commitReleaseManifest(t, scratch, manifestBytes, "")
	// Pass a DIFFERENT release version.
	exitCode, result, _, _ := runReleaseEvalManifest(t, script, "--release-version", "v0.14.0")
	if exitCode != 1 {
		t.Fatalf("override release_version mismatch must REFUSE (exit 1); got %d", exitCode)
	}
	if result.Classification != "blocker" {
		t.Errorf("override version mismatch → blocker; got %q", result.Classification)
	}
}

// TestCheckDefer_Manifest_OverrideValidAllowsAndDiscloses — a valid override
// (release_version match, all fields present, AND wrapper confirmation
// --override-confirmed-version also matches) is ACCEPTED and disclosed. The
// accepted_override appears in the output for release notes / CI publication.
// (Evaluator half of matrix case 24; the wrapper adds blob-SHA confirmation
// in Phase 2.)
func TestCheckDefer_Manifest_OverrideValidAllowsAndDiscloses(t *testing.T) {
	scratch, script, _ := setupReleaseEvalRepo(t)
	r := seededNoDiscloseInvalid("defer-ov")
	r.ReleaseRelevance = "yes"
	r.Disposition = "override_required"
	r.MetadataState = "valid"
	r.Override = &manifestOverrideSpec{
		ReleaseVersion: "v0.13.0",
		ApprovedBy:     "operator-alice",
		ApprovedAt:     "2026-07-20T00:00:00Z",
		Reason:         "Acceptable residual risk for v0.13.0; mitigation tracked separately.",
	}
	spec := manifestSpec{
		ReleaseBaseKind:     "tag",
		ReleaseBaseValue:    "v0.1.0",
		ReconciliationScope: "release arc from v0.1.0 through evaluated_commit",
		Records:             []manifestRecordSpec{r},
	}
	manifestBytes := buildManifestBytes(t, scratch, spec)
	commitReleaseManifest(t, scratch, manifestBytes, "")
	exitCode, result, stdout, _ := runReleaseEvalManifest(t, script,
		"--release-version", "v0.13.0",
		"--override-confirmed-version", "v0.13.0")
	if exitCode != 0 {
		t.Fatalf("valid override with confirmation must ALLOW (exit 0); got %d", exitCode)
	}
	if result.Classification != "disclose" {
		t.Errorf("valid override → disclose; got %q", result.Classification)
	}
	if len(result.AcceptedOverrides) != 1 {
		t.Errorf("accepted_overrides must have 1 entry; got %v", result.AcceptedOverrides)
	} else {
		ao := result.AcceptedOverrides[0]
		if ao["defer_id"] != "defer-ov" {
			t.Errorf("accepted_override.defer_id = defer-ov; got %v", ao["defer_id"])
		}
		if ao["approved_by"] != "operator-alice" {
			t.Errorf("accepted_override.approved_by must be disclosed; got %v", ao["approved_by"])
		}
	}
	if !strings.Contains(stdout, "operator-alice") || !strings.Contains(stdout, "v0.13.0") {
		t.Errorf("override must be disclosed in output (approver + version); stdout:\n%s", stdout)
	}
}

// =============================================================================
// Committed-blob authority: dirty worktree MUST NOT affect evaluation
// =============================================================================

// TestCheckDefer_Manifest_ReadsCommittedBlobNotWorktree — pins the
// manifest-authority contract that the evaluator reads the manifest bytes from
// `HEAD:<path>`, NOT from the worktree file. Without this guarantee a dirty
// worktree edit could flip a committed `block` record to `disclose+valid`
// while leaving the handshake SHAs intact, accepting a release that the
// committed manifest refuses. CI's later fresh-checkout recheck would then
// see the committed `block` and refuse publication — but the gate-protected
// release tag would already exist on the commit.
//
// Scenario:
//  1. Commit a manifest with defer-dirty = `yes+block+valid` at HEAD (full
//     handshake satisfied). The committed manifest BLOCKS.
//  2. Dirty the worktree: rewrite the manifest file (uncommitted) to flip
//     defer-dirty to `yes+disclose+valid`.
//  3. Run the evaluator in manifest mode. It MUST read the committed `block`
//     record from HEAD, NOT the dirty worktree's `disclose+valid`.
//  4. Assert: classification=blocker, exit 1, blocking_ids=[defer-dirty].
//
// Before the fix (fs.readFileSync of the worktree path): the evaluator read
// the dirty `disclose+valid` and classified `disclose` (exit 0) — the bypass.
// After the fix (git show HEAD:<path>): the evaluator reads the committed
// `block` and refuses.
func TestCheckDefer_Manifest_ReadsCommittedBlobNotWorktree(t *testing.T) {
	scratch, script, _ := setupReleaseEvalRepo(t)
	// 1. Commit a manifest with a yes+block+valid record (committed state blocks).
	r := seededNoDiscloseInvalid("defer-dirty")
	r.ReleaseRelevance = "yes"
	r.Disposition = "block"
	r.MetadataState = "valid"
	spec := manifestSpec{
		ReleaseBaseKind:     "tag",
		ReleaseBaseValue:    "v0.1.0",
		ReconciliationScope: "release arc from v0.1.0 through evaluated_commit",
		Records:             []manifestRecordSpec{r},
	}
	manifestBytes := buildManifestBytes(t, scratch, spec)
	commitReleaseManifest(t, scratch, manifestBytes, "")

	// 2. Dirty the worktree: flip the committed block record to disclose+valid.
	//    DO NOT commit. The handshake SHAs (evaluated_commit/tree/parent) must
	//    remain intact in the dirty file so the only divergence is the record
	//    body — this is the bypass scenario.
	manifestPath := filepath.Join(scratch, ".vh-agent-harness", "release-defer-dispositions.json")
	dirtyBytes, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read committed manifest: %v", err)
	}
	flipped := strings.ReplaceAll(string(dirtyBytes), `"disposition": "block"`, `"disposition": "disclose"`)
	if flipped == string(dirtyBytes) {
		t.Fatalf("setup invariant: flip block→disclose did not change manifest bytes; raw=\n%s", string(dirtyBytes))
	}
	if err := os.WriteFile(manifestPath, []byte(flipped), 0o644); err != nil {
		t.Fatalf("write dirty worktree manifest: %v", err)
	}

	// Sanity: verify the worktree is actually dirty relative to HEAD.
	gitDiff := exec.Command("git", "-C", scratch, "status", "--porcelain", "--", ".vh-agent-harness/release-defer-dispositions.json")
	diffOut, err := gitDiff.CombinedOutput()
	if err != nil {
		t.Fatalf("git status --porcelain: %v\n%s", err, diffOut)
	}
	if strings.TrimSpace(string(diffOut)) == "" {
		t.Fatalf("setup invariant: worktree manifest must be dirty relative to HEAD; got clean status")
	}

	// 3. Run the evaluator in manifest-authority mode.
	exitCode, result, _, _ := runReleaseEvalManifest(t, script)

	// 4. Assert: REFUSAL — the committed `block` was read from HEAD, NOT the
	//    dirty worktree's `disclose+valid`. Before the fix this returned exit 0
	//    (disclose), demonstrating the bypass.
	if exitCode != 1 {
		t.Fatalf("dirty-worktree bypass must REFUSE (exit 1, blocker); got exit=%d (classification=%q err=%v)",
			exitCode, result.Classification, result.Error)
	}
	if result.Classification != "blocker" {
		t.Fatalf("committed block must classify as blocker (read from HEAD); got %q (err=%v)",
			result.Classification, result.Error)
	}
	if len(result.BlockingIDs) != 1 || result.BlockingIDs[0] != "defer-dirty" {
		t.Errorf("blocking_ids must be [defer-dirty]; got %v", result.BlockingIDs)
	}
}

// =============================================================================
// End-to-end seed demonstration: the actual defer-p1-lineage-002-001 shape
// =============================================================================

// TestCheckDefer_Manifest_SeedRecordShapeUnblocks — the canonical
// defer-p1-lineage-002-001 disposition (no+disclose+invalid) is carried by
// the committed manifest and ALLOWS the release while disclosing the record.
// This is the "unblock" path: the broken trigger grammar in the .local/ card
// (which would be evaluator-error in legacy release mode) is NOT release-
// relevant and is classified disclose+invalid in the manifest. The release
// proceeds with disclosure. Also demonstrates manifest mode works with .local/
// absent.
func TestCheckDefer_Manifest_SeedRecordShapeUnblocks(t *testing.T) {
	scratch, script, _ := setupReleaseEvalRepo(t)
	spec := manifestSpec{
		ReleaseBaseKind:     "tag",
		ReleaseBaseValue:    "v0.1.0",
		ReconciliationScope: "release arc from v0.1.0 through evaluated_commit",
		Records: []manifestRecordSpec{
			{
				DeferID:          "defer-p1-lineage-002-001",
				ReleaseRelevance: "no",
				Disposition:      "disclose",
				MetadataState:    "invalid",
				Summary:          "Conditional future prune-phase coverage follow-up.",
				Reason:           "Not part of the current release; retained for audit with unsupported legacy trigger metadata.",
				SourceRef:        ".local/coordinator/tasks/defer-p1-lineage-002-001.json",
				StudiedAt:        "2026-07-15T00:00:00Z",
				ReviewedAt:       "2026-07-20T00:00:00Z",
			},
		},
	}
	manifestBytes := buildManifestBytes(t, scratch, spec)
	commitReleaseManifest(t, scratch, manifestBytes, "")
	exitCode, result, stdout, _ := runReleaseEvalManifest(t, script)
	if exitCode != 0 {
		t.Fatalf("seed shape must ALLOW release (exit 0); got %d (err=%v)", exitCode, result.Error)
	}
	if result.Classification != "disclose" {
		t.Errorf("seed shape → disclose; got %q", result.Classification)
	}
	if !strings.Contains(stdout, "defer-p1-lineage-002-001") {
		t.Errorf("seed record must be disclosed in output; stdout:\n%s", stdout)
	}
	// manifest_sha must be populated (for wrapper override binding in Phase 2).
	if result.ManifestSHA == "" {
		t.Errorf("manifest_sha must be populated; got empty")
	}
	if !regexpMatch(`^[0-9a-f]{40}$`, result.ManifestSHA) {
		t.Errorf("manifest_sha must be a 40-char SHA; got %q", result.ManifestSHA)
	}
}

// regexpMatch is a tiny helper to avoid importing the regexp package at the
// top level just for one SHA check.
func regexpMatch(pattern, s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		ok := (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')
		if !ok {
			return false
		}
	}
	return len(s) == 40
}
