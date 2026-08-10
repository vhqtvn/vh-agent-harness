package cli

// doctor_redlines_test.go tests the applicability-gated private-redlines doctor
// hygiene check (internal/cli/doctor_redlines.go). All fixtures use OBVIOUSLY
// synthetic terms. Every test that uses synthetic terms asserts no-leak over the
// diagnostic output (paste-safe contract).

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// redlinesDoctorOut runs the FULL runDoctor against abs and returns its output,
// exactly like seamDoctorOut but without requiring a seam install (the redlines
// check is install-independent; other checks may FAIL on a bare dir and that is
// fine — we only assert on the redlines section).
func redlinesDoctorOut(t *testing.T, abs string) string {
	t.Helper()
	var out string
	runWithCwd(t, abs, func() {
		doctorTargetFlag = abs
		defer func() { doctorTargetFlag = "" }()
		cmd, buf := newOutCmd()
		_ = runDoctor(cmd, []string{})
		out = buf.String()
	})
	return out
}

// writeUserRegMode writes content as the user-level registry with an explicit
// mode (for the insecure-mode test). writeRedlinesUserReg writes 0600.
func writeUserRegMode(t *testing.T, content string, mode os.FileMode) {
	t.Helper()
	xdg := os.Getenv("XDG_CONFIG_HOME")
	if xdg == "" {
		t.Fatal("test precondition: XDG_CONFIG_HOME not set")
	}
	regDir := filepath.Join(xdg, "vh-agent-harness", "redlines")
	if err := os.MkdirAll(regDir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", regDir, err)
	}
	path := filepath.Join(regDir, "registry.yml")
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatalf("write registry: %v", err)
	}
	if err := os.Chmod(path, mode); err != nil { // force exact mode (umask-safe)
		t.Fatalf("chmod registry: %v", err)
	}
}

// allSyntheticTerms is the full set of configured terms used across these
// fixtures. assertNoLeak verifies NONE appear in diagnostic output.
var allSyntheticTerms = []string{
	"synthetic-scrub-vocab",
	"synthetic-org-alpha",
	"synthetic-domain-beta",
	"synthetic-ambient-gamma",
	"synthetic-ambient-org",
	"synthetic-scrub-rationale",
	"synthetic-relation-rationale",
}

// --- (a) registry absent → NO section (the critical zero-footprint test) ---

// TestDoctorRedlines_NoRegistryOmitsSection: with NO user-level registry, doctor
// must emit NO redlines section at all — no warning, no output line, zero
// footprint for non-adopters. This is the load-bearing applicability invariant.
func TestDoctorRedlines_NoRegistryOmitsSection(t *testing.T) {
	xdg, xdgCleanup := setRedlinesXDG(t)
	defer xdgCleanup()
	repoDir := t.TempDir()

	// No registry written. Snapshot file counts to prove zero footprint.
	assertRedlinesNoNewFiles(t, xdg, "XDG dir")
	assertRedlinesNoNewFiles(t, repoDir, "repo dir")

	out := redlinesDoctorOut(t, repoDir)
	if strings.Contains(out, "redlines:") || strings.Contains(out, "redlines ") {
		t.Errorf("no-registry: doctor must NOT emit a redlines section (zero footprint); got:\n%s", out)
	}
	// The check name must not appear anywhere as a diagnostic line.
	for _, banned := range []string{"redlines PASS", "redlines WARN", "redlines FAIL", "redlines SKIP"} {
		if strings.Contains(out, banned) {
			t.Errorf("no-registry: doctor must NOT emit any redlines diagnostic; found %q in:\n%s", banned, out)
		}
	}
}

// --- direct unit check: registry absent → SKIP (defensive re-check) ---

func TestCheckPrivateRedlines_NoRegistrySkips(t *testing.T) {
	setRedlinesXDG(t)
	repoDir := t.TempDir()
	r := checkPrivateRedlines(repoDir)
	if r.tier != tierSkip {
		t.Errorf("no registry: want tier SKIP, got %s (detail=%s)", r.tier, r.detail)
	}
}

// --- (b) registry present + valid + binding → PASS with opaque ids ---

func TestCheckPrivateRedlines_PresentValidBinding_PASS(t *testing.T) {
	setRedlinesXDG(t)
	writeRedlinesUserReg(t, `version: 1
subjects:
  - id: subj-test-scrub
    kind: scrub-project
    labels: [synthetic-scrub-vocab]
    why: synthetic-scrub-rationale
  - id: subj-test-rel
    kind: forbidden-relation
    side_a: [synthetic-org-alpha]
    side_b: [synthetic-domain-beta]
`)
	repoDir := t.TempDir()
	gitInit(t, repoDir)
	gitConfigTest(t, repoDir)

	r := checkPrivateRedlines(repoDir)
	if r.tier != tierPass {
		t.Errorf("valid binding registry: want PASS, got %s (detail=%s)", r.tier, r.detail)
	}
	// Opaque IDs must appear (paste-safe).
	for _, want := range []string{"subj-test-scrub", "subj-test-rel", "2 subject(s) bind"} {
		if !strings.Contains(r.detail, want) {
			t.Errorf("PASS detail missing %q; got: %s", want, r.detail)
		}
	}
	// NO configured term may leak.
	assertNoLeak(t, r.detail, "valid-binding", "detail", allSyntheticTerms...)
	assertNoLeak(t, r.String(), "valid-binding", "String()", allSyntheticTerms...)
}

// --- integration: registry present → section appears in full doctor output ---

func TestDoctorRedlines_PresentRegistryShowsSection(t *testing.T) {
	setRedlinesXDG(t)
	writeRedlinesUserReg(t, `version: 1
subjects:
  - id: subj-test-scrub
    kind: scrub-project
    labels: [synthetic-scrub-vocab]
`)
	repoDir := t.TempDir()
	out := redlinesDoctorOut(t, repoDir)
	if !strings.Contains(out, "redlines:") {
		t.Errorf("registry present: doctor must show a redlines section; got:\n%s", out)
	}
	// Section must name the opaque id and stay paste-safe.
	if !strings.Contains(out, "subj-test-scrub") {
		t.Errorf("redlines section should name the opaque subject id; got:\n%s", out)
	}
	assertNoLeak(t, out, "present-registry", "doctor output", allSyntheticTerms...)
}

// --- (c) registry present + invalid → WARN opaque error ---

func TestCheckPrivateRedlines_PresentInvalid_WARN(t *testing.T) {
	setRedlinesXDG(t)
	// Non-opaque id triggers a fail-closed validation error (no term echo).
	writeRedlinesUserReg(t, `version: 1
subjects:
  - id: not-opaque-id
    kind: scrub-project
    labels: [synthetic-scrub-vocab]
`)
	repoDir := t.TempDir()
	r := checkPrivateRedlines(repoDir)
	if r.tier != tierWarn {
		t.Errorf("invalid registry: want WARN, got %s (detail=%s)", r.tier, r.detail)
	}
	// The WARN must name the problem (unreadable/invalid) without echoing terms.
	if !strings.Contains(r.detail, "unreadable/invalid") && !strings.Contains(r.detail, "invalid") {
		t.Errorf("invalid-registry WARN should state unreadable/invalid; got: %s", r.detail)
	}
	assertNoLeak(t, r.detail, "invalid", "detail", allSyntheticTerms...)
	assertNoLeak(t, r.String(), "invalid", "String()", allSyntheticTerms...)
}

// --- (d) registry present + insecure mode → WARN ---

func TestCheckPrivateRedlines_InsecureMode_WARN(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "" {
		t.Skip("skipping POSIX mode test in helper process")
	}
	setRedlinesXDG(t)
	writeUserRegMode(t, `version: 1
subjects:
  - id: subj-test-scrub
    kind: scrub-project
    labels: [synthetic-scrub-vocab]
`, 0o644) // group/world-readable — the WARN condition
	repoDir := t.TempDir()
	r := checkPrivateRedlines(repoDir)
	if r.tier != tierWarn {
		t.Errorf("insecure mode: want WARN, got %s (detail=%s)", r.tier, r.detail)
	}
	if !strings.Contains(r.detail, "group/world-readable") {
		t.Errorf("insecure-mode WARN should name group/world-readable; got: %s", r.detail)
	}
	assertNoLeak(t, r.detail, "insecure-mode", "detail", allSyntheticTerms...)
}

// --- (e) repo-local redlines file tracked → FAIL ---

func TestCheckPrivateRedlines_RepoLocalTracked_FAIL(t *testing.T) {
	setRedlinesXDG(t)
	writeRedlinesUserReg(t, `version: 1
subjects:
  - id: subj-test-scrub
    kind: scrub-project
    labels: [synthetic-scrub-vocab]
`)
	repoDir := t.TempDir()
	gitInit(t, repoDir)
	gitConfigTest(t, repoDir)
	// Write the repo-local additive file and TRACK it (the leak condition).
	writeRepoFile(t, repoDir, ".vh-agent-harness/redlines.local.yml",
		"version: 1\nsubjects: []\n")
	gitRun(t, repoDir, "add", ".vh-agent-harness/redlines.local.yml")

	r := checkPrivateRedlines(repoDir)
	if r.tier != tierFail {
		t.Errorf("tracked repo-local: want FAIL, got %s (detail=%s)", r.tier, r.detail)
	}
	if !strings.Contains(r.detail, "TRACKED") {
		t.Errorf("tracked repo-local FAIL should name TRACKED; got: %s", r.detail)
	}
	assertNoLeak(t, r.detail, "tracked-repolocal", "detail", allSyntheticTerms...)
}

// --- repo-local present + NOT gitignored → FAIL (would be staged) ---

func TestCheckPrivateRedlines_RepoLocalPresentUnignored_FAIL(t *testing.T) {
	setRedlinesXDG(t)
	writeRedlinesUserReg(t, `version: 1
subjects:
  - id: subj-test-scrub
    kind: scrub-project
    labels: [synthetic-scrub-vocab]
`)
	repoDir := t.TempDir()
	gitInit(t, repoDir)
	gitConfigTest(t, repoDir)
	// Write the repo-local file but leave it UNTRACKED and NOT gitignored.
	writeRepoFile(t, repoDir, ".vh-agent-harness/redlines.local.yml",
		"version: 1\nsubjects: []\n")

	r := checkPrivateRedlines(repoDir)
	if r.tier != tierFail {
		t.Errorf("unignored repo-local: want FAIL, got %s (detail=%s)", r.tier, r.detail)
	}
	if !strings.Contains(r.detail, "NOT gitignored") {
		t.Errorf("unignored repo-local FAIL should name NOT gitignored; got: %s", r.detail)
	}
	assertNoLeak(t, r.detail, "unignored-repolocal", "detail", allSyntheticTerms...)
}

// --- repo-local properly gitignored → no tracked/unignored finding ---

func TestCheckPrivateRedlines_RepoLocalGitignoredIsClean(t *testing.T) {
	setRedlinesXDG(t)
	writeRedlinesUserReg(t, `version: 1
subjects:
  - id: subj-test-scrub
    kind: scrub-project
    labels: [synthetic-scrub-vocab]
`)
	repoDir := t.TempDir()
	gitInit(t, repoDir)
	gitConfigTest(t, repoDir)
	writeRepoFile(t, repoDir, ".gitignore", ".vh-agent-harness/redlines.local.yml\n")
	writeRepoFile(t, repoDir, ".vh-agent-harness/redlines.local.yml",
		"version: 1\nsubjects: []\n")
	// Tighten to 0600 so the insecure-mode sub-check does not WARN (this test
	// isolates the tracked/ignored sub-check, not mode exposure).
	rlPath := filepath.Join(repoDir, ".vh-agent-harness", "redlines.local.yml")
	if err := os.Chmod(rlPath, 0o600); err != nil {
		t.Fatalf("chmod repo-local: %v", err)
	}
	gitRun(t, repoDir, "add", ".gitignore")
	gitRun(t, repoDir, "commit", "-q", "-m", "gitignore")

	r := checkPrivateRedlines(repoDir)
	// With a portable repo .gitignore rule, no tracked/unignored finding.
	// Healthy state = PASS with binding summary.
	if r.tier != tierPass {
		t.Errorf("gitignored repo-local: want PASS, got %s (detail=%s)", r.tier, r.detail)
	}
	if strings.Contains(r.detail, "TRACKED") || strings.Contains(r.detail, "NOT gitignored") {
		t.Errorf("gitignored repo-local should not flag tracked/unignored; got: %s", r.detail)
	}
	assertNoLeak(t, r.detail, "gitignored-repolocal", "detail", allSyntheticTerms...)
}

// --- non-binding registry → PASS with "0 subjects bind" (inert here) ---

func TestCheckPrivateRedlines_NonBindingRegistry_PASS(t *testing.T) {
	setRedlinesXDG(t)
	writeRedlinesUserReg(t, `version: 1
subjects:
  - id: subj-test-scrub
    kind: scrub-project
    labels: [synthetic-scrub-vocab]
    repos: ["/totally/elsewhere/**"]
`)
	repoDir := t.TempDir()
	r := checkPrivateRedlines(repoDir)
	if r.tier != tierPass {
		t.Errorf("non-binding: want PASS, got %s (detail=%s)", r.tier, r.detail)
	}
	if !strings.Contains(r.detail, "0 subjects bind") {
		t.Errorf("non-binding PASS should state 0 subjects bind; got: %s", r.detail)
	}
	// The opaque id must NOT appear (the subject does not bind).
	if strings.Contains(r.detail, "subj-test-scrub") {
		t.Errorf("non-binding must not list the unbound subject id; got: %s", r.detail)
	}
	assertNoLeak(t, r.detail, "non-binding", "detail", allSyntheticTerms...)
}
