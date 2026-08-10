package cli

// commit_gate_redlines_test.go black-boxes the private-redlines integration in
// the RENDERED .opencode/scripts/commit-gate.sh. After `git write-tree` builds
// the private staged tree and before acquire completion, the gate invokes
// `vh-agent-harness redlines scan -C <repo-root> --tree <hash>`:
//   - exit 0  -> pass / non-applicable; captured output DISCARDED (zero-footprint)
//   - exit 1  -> BLOCK (status redlines_violation)
//   - exit 2  -> BLOCK fail-closed (status redlines_error)
//   - binary missing on PATH -> silent skip (redlines is a feature of an
//     installed harness; no binary means not-applicable)
//
// All fixtures use OBVIOUSLY synthetic terms only (termScrub / termSideA /
// termSideB from redlines_scan_test.go; subj-test-* ids). No real terms appear
// in any fixture, registry, or assertion.

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// redlinesGateSetup creates an isolated scratch git repo with the RENDERED gate
// scripts (commit-gate.sh + readonly-scripts.sh + validate-opencode-config.py)
// and a minimal valid opencode config (so _config_validate passes), then seeds
// git identity. It returns (repoDir, scriptsDir). It does NOT seed any tracked
// file — callers stage the acquire payload themselves.
func redlinesGateSetup(t *testing.T) (string, string) {
	t.Helper()
	repoRoot := findModuleRoot(t)
	srcScripts := filepath.Join(repoRoot, ".opencode", "scripts")
	for _, f := range []string{"commit-gate.sh", "readonly-scripts.sh", "validate-opencode-config.py"} {
		if _, err := os.Stat(filepath.Join(srcScripts, f)); err != nil {
			t.Skipf("rendered script %s unavailable: %v (run `make update` first)", f, err)
		}
	}
	dir := t.TempDir()
	dstScripts := filepath.Join(dir, ".opencode", "scripts")
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
	gitInit(t, dir)
	gitConfigTest(t, dir)
	return dir, dstScripts
}

// redlinesGateAcquire drives `commit-gate.sh acquire` with a fresh message file
// and the given paths (JSON array string). env may be nil (inherit the process
// env, so an ensureHarnessBinaryOnPath binary + XDG propagate) or a custom env
// slice (for the binary-missing case). It returns (combinedOutput, parsedStatus,
// detail) where detail is the paste-safe scanner output the gate embedded in the
// JSON "detail" field (empty on exit-0 / skip).
func redlinesGateAcquire(t *testing.T, dir, scriptsDir, pathsJSON, msgBody string, env []string) (string, map[string]any, string) {
	t.Helper()
	uuidOut, err := exec.Command("bash", filepath.Join(scriptsDir, "readonly-scripts.sh"), "gen-uuid").Output()
	if err != nil {
		t.Fatalf("gen-uuid: %v", err)
	}
	uuidA := strings.TrimSpace(string(uuidOut))
	msgRel := filepath.ToSlash(filepath.Join("tmp", "commit-gate-message", "msg-"+uuidA))
	msgFull := filepath.Join(dir, filepath.FromSlash(msgRel))
	if err := os.MkdirAll(filepath.Dir(msgFull), 0o755); err != nil {
		t.Fatalf("mkdir scratch: %v", err)
	}
	if err := os.WriteFile(msgFull, []byte(msgBody), 0o644); err != nil {
		t.Fatalf("write msg: %v", err)
	}
	args := []string{filepath.Join(scriptsDir, "commit-gate.sh"), "acquire",
		"--paths", pathsJSON,
		"--message-file", msgRel,
		"--session-alias", "redlines-gate-test"}
	cmd := exec.Command("bash", args...)
	cmd.Dir = dir
	if env != nil {
		cmd.Env = env
	}
	out, _ := cmd.CombinedOutput()
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
	detail, _ := parsed["detail"].(string)
	return combined, parsed, detail
}

// pathWithoutHarnessEnv returns an env (os.Environ() shape) identical to the
// process env except PATH has every directory containing a `vh-agent-harness`
// binary removed. Used by the binary-missing case so the gate's
// `command -v vh-agent-harness` fails regardless of whether an earlier test in
// the same package called ensureHarnessBinaryOnPath (which leaks a global
// PATH prepend). git/bash/python3 dirs are preserved.
func pathWithoutHarnessEnv(t *testing.T) []string {
	t.Helper()
	src := os.Environ()
	out := make([]string, 0, len(src))
	var pathEntry string
	for _, e := range src {
		if strings.HasPrefix(e, "PATH=") {
			pathEntry = e
		} else {
			out = append(out, e)
		}
	}
	dirs := filepath.SplitList(strings.TrimPrefix(pathEntry, "PATH="))
	kept := make([]string, 0, len(dirs))
	for _, d := range dirs {
		if d != "" {
			if _, err := os.Stat(filepath.Join(d, "vh-agent-harness")); err == nil {
				continue // exclude any dir holding the harness binary
			}
		}
		kept = append(kept, d)
	}
	out = append(out, "PATH="+strings.Join(kept, string(os.PathListSeparator)))
	return out
}

// --- zero-footprint / pass cases ---

// Case 1: no registry at all -> acquire succeeds, ZERO footprint (the word
// "redlines" must not appear anywhere in the gate output).
func TestCommitGate_Redlines_NoRegistryZeroFootprint(t *testing.T) {
	ensureHarnessBinaryOnPath(t)
	setRedlinesXDG(t)
	dir, scripts := redlinesGateSetup(t)
	writeRepoFile(t, dir, "notes.txt", "clean content\n")
	combined, parsed, _ := redlinesGateAcquire(t, dir, scripts, `["notes.txt"]`, "add notes\n", nil)
	if got := statusOf(parsed); got != "acquired" {
		t.Fatalf("no registry: want acquired, got %q\n%s", got, combined)
	}
	if strings.Contains(combined, "redlines") {
		t.Errorf("no registry: zero-footprint violated — output must not mention redlines:\n%s", combined)
	}
}

// Case 2: registry exists but binds nothing in this repo -> acquire succeeds,
// zero-footprint (scan exits 0 with a status line; the gate DISCARDS it).
func TestCommitGate_Redlines_NonBindingRegistryPasses(t *testing.T) {
	ensureHarnessBinaryOnPath(t)
	setRedlinesXDG(t)
	writeRedlinesUserReg(t, `version: 1
subjects:
  - id: subj-test-scrub
    kind: scrub-project
    labels: [`+termScrub+`]
    repos: ["/totally/elsewhere/**"]
`)
	dir, scripts := redlinesGateSetup(t)
	writeRepoFile(t, dir, "clean.txt", "clean\n")
	combined, parsed, _ := redlinesGateAcquire(t, dir, scripts, `["clean.txt"]`, "add\n", nil)
	if got := statusOf(parsed); got != "acquired" {
		t.Fatalf("non-binding: want acquired, got %q\n%s", got, combined)
	}
	if strings.Contains(combined, "redlines") {
		t.Errorf("non-binding: zero-footprint violated:\n%s", combined)
	}
}

// Case 3: binding registry, but the staged tree is clean -> acquire succeeds,
// zero-footprint (scan finds nothing; honesty pointer is discarded).
func TestCommitGate_Redlines_BindingCleanTreePasses(t *testing.T) {
	ensureHarnessBinaryOnPath(t)
	setRedlinesXDG(t)
	writeRedlinesUserReg(t, `version: 1
subjects:
  - id: subj-test-scrub
    kind: scrub-project
    labels: [`+termScrub+`]
`)
	dir, scripts := redlinesGateSetup(t)
	writeRepoFile(t, dir, "clean.txt", "no issues here\n")
	combined, parsed, _ := redlinesGateAcquire(t, dir, scripts, `["clean.txt"]`, "add\n", nil)
	if got := statusOf(parsed); got != "acquired" {
		t.Fatalf("binding clean: want acquired, got %q\n%s", got, combined)
	}
	if strings.Contains(combined, "redlines") {
		t.Errorf("binding clean: zero-footprint violated:\n%s", combined)
	}
}

// --- block cases ---

// Case 4: staged tree contains a scrub-project violation -> BLOCK, opaque
// detail (subj-id + reason + path), NO configured term leak.
func TestCommitGate_Redlines_ScrubViolationBlocks(t *testing.T) {
	ensureHarnessBinaryOnPath(t)
	setRedlinesXDG(t)
	writeRedlinesUserReg(t, `version: 1
subjects:
  - id: subj-test-scrub
    kind: scrub-project
    labels: [`+termScrub+`]
`)
	dir, scripts := redlinesGateSetup(t)
	writeRepoFile(t, dir, "secret.md", "this mentions "+termScrub+" in passing\n")
	combined, parsed, detail := redlinesGateAcquire(t, dir, scripts, `["secret.md"]`, "add secret\n", nil)
	if got := statusOf(parsed); got != "redlines_violation" {
		t.Fatalf("scrub violation: want redlines_violation, got %q\n%s", got, combined)
	}
	for _, want := range []string{"subj-test-scrub", "scrub-term", "secret.md"} {
		if !strings.Contains(detail, want) {
			t.Errorf("scrub violation: detail missing %q:\n%s", want, detail)
		}
	}
	assertNoLeak(t, combined, "gate-scrub", "combined", termScrub)
}

// Case 5: staged tree contains a forbidden-relation co-occurrence -> BLOCK,
// opaque detail, NO configured side leak.
func TestCommitGate_Redlines_RelationCoOccurrenceBlocks(t *testing.T) {
	ensureHarnessBinaryOnPath(t)
	setRedlinesXDG(t)
	writeRedlinesUserReg(t, `version: 1
subjects:
  - id: subj-test-rel
    kind: forbidden-relation
    side_a: [`+termSideA+`]
    side_b: [`+termSideB+`]
    unit: file
`)
	dir, scripts := redlinesGateSetup(t)
	writeRepoFile(t, dir, "doc.txt", "project uses "+termSideA+" with "+termSideB+"\n")
	combined, parsed, detail := redlinesGateAcquire(t, dir, scripts, `["doc.txt"]`, "add doc\n", nil)
	if got := statusOf(parsed); got != "redlines_violation" {
		t.Fatalf("relation: want redlines_violation, got %q\n%s", got, combined)
	}
	for _, want := range []string{"subj-test-rel", "relation-co-occurrence", "doc.txt"} {
		if !strings.Contains(detail, want) {
			t.Errorf("relation: detail missing %q:\n%s", want, detail)
		}
	}
	assertNoLeak(t, combined, "gate-relation", "combined", termSideA, termSideB)
}

// Case 5b: staged tree contains ONLY a side_b term in a repo bound as AMBIENT
// (repo identity implies side_a) -> BLOCK, opaque detail, NO configured side
// leak. This covers the ambient degeneration path at the commit-gate
// integration layer. The scan + engine layers already cover it
// (TestRedlinesScan_RelationAmbientViolation / TestScan_RelationAmbientSideBAloneHits);
// this test closes the integration-layer gap the reviewer flagged.
func TestCommitGate_Redlines_RelationAmbientViolationBlocks(t *testing.T) {
	ensureHarnessBinaryOnPath(t)
	setRedlinesXDG(t)
	dir, scripts := redlinesGateSetup(t)
	// Configure AFTER setup so ambient_repos can reference the repo dir the
	// gate runs in. The repo has no git remotes (gitInit only), so ambient
	// binding is established via the path glob.
	writeRedlinesUserReg(t, `version: 1
subjects:
  - id: subj-test-amb
    kind: forbidden-relation
    side_a: [`+termAmbientA+`]
    side_b: [`+termAmbientB+`]
    ambient_repos: ["`+dir+`**"]
`)
	// Stage a file containing ONLY the side_b term (no side_a). Ambient means
	// side_a is implied by repo identity, so side_b alone is a violation.
	writeRepoFile(t, dir, "config.yml", "endpoint: "+termAmbientB+"\n")
	combined, parsed, detail := redlinesGateAcquire(t, dir, scripts, `["config.yml"]`, "add ambient\n", nil)
	if got := statusOf(parsed); got != "redlines_violation" {
		t.Fatalf("ambient: want redlines_violation, got %q\n%s", got, combined)
	}
	for _, want := range []string{"subj-test-amb", "relation-ambient-side-b", "config.yml"} {
		if !strings.Contains(detail, want) {
			t.Errorf("ambient: detail missing %q:\n%s", want, detail)
		}
	}
	assertNoLeak(t, combined, "gate-ambient", "combined", termAmbientA, termAmbientB)
}

// Case 6: binding registry that is applicable-but-invalid (non-opaque subject
// id) -> BLOCK fail-closed (status redlines_error), NO term leak.
func TestCommitGate_Redlines_InvalidRegistryFailsClosed(t *testing.T) {
	ensureHarnessBinaryOnPath(t)
	setRedlinesXDG(t)
	// Invalid: subject id is not opaque (fails the subj-* id validation) —
	// Load errors and scan exits 2 (fail-closed).
	writeRedlinesUserReg(t, `version: 1
subjects:
  - id: not-opaque-id
    kind: scrub-project
    labels: [`+termScrub+`]
`)
	dir, scripts := redlinesGateSetup(t)
	writeRepoFile(t, dir, "clean.txt", "clean\n")
	combined, parsed, _ := redlinesGateAcquire(t, dir, scripts, `["clean.txt"]`, "add\n", nil)
	if got := statusOf(parsed); got != "redlines_error" {
		t.Fatalf("invalid registry: want redlines_error (fail-closed), got %q\n%s", got, combined)
	}
	assertNoLeak(t, combined, "gate-invalid", "combined", termScrub)
}

// Case 7: the scan targets the EXACT staged tree, not the working tree. A
// violation is staged (acquired); a DIFFERENT violation sits only in the working
// tree, unstaged. The gate BLOCKS on the staged one and the unstaged path must
// NOT appear in the detail (no fallback to the working tree).
func TestCommitGate_Redlines_ExactTreeIsolation(t *testing.T) {
	ensureHarnessBinaryOnPath(t)
	setRedlinesXDG(t)
	writeRedlinesUserReg(t, `version: 1
subjects:
  - id: subj-test-scrub
    kind: scrub-project
    labels: [`+termScrub+`]
`)
	dir, scripts := redlinesGateSetup(t)
	// Staged violation (in the acquire path list).
	writeRepoFile(t, dir, "staged_secret.md", "leaked "+termScrub+"\n")
	// Unstaged violation (working-tree only, NOT in the acquire path list).
	writeRepoFile(t, dir, "unstaged_secret.md", "leaked "+termScrub+"\n")
	combined, parsed, detail := redlinesGateAcquire(t, dir, scripts, `["staged_secret.md"]`, "add staged\n", nil)
	if got := statusOf(parsed); got != "redlines_violation" {
		t.Fatalf("isolation: want redlines_violation (staged tree has a violation), got %q\n%s", got, combined)
	}
	if !strings.Contains(detail, "staged_secret.md") {
		t.Errorf("isolation: detail must reference the STAGED violation path:\n%s", detail)
	}
	if strings.Contains(detail, "unstaged_secret.md") {
		t.Errorf("isolation: detail must NOT reference the unstaged (working-tree-only) path — scan fell back to the working tree:\n%s", detail)
	}
}

// Case 8: blocking detail (JSON detail field + stderr) is paste-safe even when
// the registry carries a `why:` rationale — opaque id/reason/path present, the
// configured term AND the why rationale NEVER appear.
func TestCommitGate_Redlines_BlockDetailPasteSafe(t *testing.T) {
	ensureHarnessBinaryOnPath(t)
	setRedlinesXDG(t)
	const why = "private-scrub-rationale-text"
	writeRedlinesUserReg(t, `version: 1
subjects:
  - id: subj-test-scrub
    kind: scrub-project
    labels: [`+termScrub+`]
    why: `+why+`
`)
	dir, scripts := redlinesGateSetup(t)
	writeRepoFile(t, dir, "secret.md", "mentions "+termScrub+"\n")
	combined, parsed, detail := redlinesGateAcquire(t, dir, scripts, `["secret.md"]`, "add\n", nil)
	if got := statusOf(parsed); got != "redlines_violation" {
		t.Fatalf("paste-safe: want redlines_violation, got %q\n%s", got, combined)
	}
	for _, want := range []string{"subj-test-scrub", "scrub-term", "secret.md"} {
		if !strings.Contains(detail, want) {
			t.Errorf("paste-safe: detail missing %q:\n%s", want, detail)
		}
	}
	// Both the structured detail and the full combined output must omit the
	// configured term and the private why rationale.
	assertNoLeak(t, detail, "gate-paste-safe", "detail", termScrub, why)
	assertNoLeak(t, combined, "gate-paste-safe", "combined", termScrub, why)
}

// --- binary-missing case (locks in FLAG B: silent skip) ---

// Case 9: `vh-agent-harness` is NOT on PATH -> the redlines check is a silent
// no-op and acquire succeeds, even when a binding registry + a staged violation
// WOULD block if the binary were present. The subprocess PATH is stripped of
// every harness-containing dir so this is robust regardless of test order.
// ensureHarnessBinaryOnPath is deliberately NOT called.
func TestCommitGate_Redlines_BinaryMissingSkipsSilently(t *testing.T) {
	setRedlinesXDG(t)
	writeRedlinesUserReg(t, `version: 1
subjects:
  - id: subj-test-scrub
    kind: scrub-project
    labels: [`+termScrub+`]
`)
	dir, scripts := redlinesGateSetup(t)
	writeRepoFile(t, dir, "secret.md", "mentions "+termScrub+"\n")
	env := pathWithoutHarnessEnv(t)
	combined, parsed, _ := redlinesGateAcquire(t, dir, scripts, `["secret.md"]`, "add\n", env)
	if got := statusOf(parsed); got != "acquired" {
		t.Fatalf("binary missing: want acquired (redlines silently skipped), got %q\n%s", got, combined)
	}
	if strings.Contains(combined, "redlines") {
		t.Errorf("binary missing: output must not mention redlines (silent skip):\n%s", combined)
	}
}
