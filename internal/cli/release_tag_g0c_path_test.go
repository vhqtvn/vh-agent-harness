package cli

// release_tag_g0c_path_test.go — behavioral-closure crux for the §4.7
// stale-PATH-binary recurrence correction (Layer 1: G0c resolves
// ./bin/vh-agent-harness deterministically, NEVER PATH).
//
// The defect class: a stale PATH-resolved vh-agent-harness binary halted THREE
// consecutive releases (v0.15.0 dev-stale-binary, v0.17.0 residual reinstall,
// v0.18.0 STOP-AND-ASK #1) for a mechanical, decision-free recovery. After the
// Layer 1 pin, the wrapper's G0c gate resolves the ceremony-local binary
// (./bin/vh-agent-harness, produced by `make build` at ceremony start) and
// ignores whatever is on PATH.
//
// This file proves that outcome: a ceremony whose PATH resolves a DECOY stale
// binary (the decoy would halt the ceremony under the old PATH-resolved code)
// now runs G0c against the fresh ceremony binary. The decoy is provably never
// invoked — the refuse (doctor unhealthy in the scratch fixture, a legitimate
// red) carries real doctor output from the FRESH binary, never the decoy's
// marker and never the old "stale binary on PATH" message.

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// decoyBinaryMarker is the unique string the decoy stale binary prints to
// stderr on every invocation. If G0c ever resolved PATH (a regression), this
// marker would appear in the captured doctor output OR the wrapper would refuse
// with the old "stale binary on PATH" canary message. After Layer 1, the marker
// never appears because G0c resolves ./bin/vh-agent-harness.
const decoyBinaryMarker = "G0C_PATH_DECOY_STALE_BINARY_WAS_RESOLVED_FROM_PATH"

// writeDecoyStaleBinary writes a decoy `vh-agent-harness` into dir that, on ANY
// invocation, prints decoyBinaryMarker to stderr/stdout and exits non-zero. If
// the wrapper resolved this from PATH, the G0c canary (`release inject-errata
// --help`) would fail and the wrapper would refuse with the stale-binary
// message — the defect. Placing the decoy's dir FIRST on PATH makes it WIN PATH
// resolution, so the only way the ceremony avoids it is by resolving the
// ceremony-local ./bin/vh-agent-harness instead.
func writeDecoyStaleBinary(t *testing.T, dir string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir decoy dir: %v", err)
	}
	decoy := filepath.Join(dir, "vh-agent-harness")
	body := "#!/bin/sh\n" +
		"echo '" + decoyBinaryMarker + "' >&2\n" +
		"echo '" + decoyBinaryMarker + "'\n" +
		"exit 1\n"
	if err := os.WriteFile(decoy, []byte(body), 0o755); err != nil {
		t.Fatalf("write decoy: %v", err)
	}
	return decoy
}

// envWithPATHFirst builds a cmd.Env slice identical to os.Environ() except PATH
// is replaced with `first` prepended, so the decoy dir WINS PATH resolution.
func envWithPATHFirst(t *testing.T, first string) []string {
	t.Helper()
	base := os.Environ()
	out := make([]string, 0, len(base)+1)
	for _, kv := range base {
		if strings.HasPrefix(kv, "PATH=") {
			continue
		}
		out = append(out, kv)
	}
	orig := os.Getenv("PATH")
	out = append(out, "PATH="+first+string(os.PathListSeparator)+orig)
	return out
}

// runG0cCeremony invokes the wrapper with the decoy dir FIRST on PATH. cwd is
// <scratch>. Returns (exitCode, stdout, stderr).
func runG0cCeremony(t *testing.T, wrapper, msgFile, version, decoyDir string) (int, string, string) {
	t.Helper()
	args := []string{wrapper, version}
	cmd := exec.Command("bash", args...)
	cmd.Dir = filepath.Dir(filepath.Dir(wrapper)) // <scratch>
	cmd.Env = append(envWithPATHFirst(t, decoyDir), "RELEASE_TAG_MESSAGE_FILE="+msgFile)
	var outb, errb strings.Builder
	cmd.Stdout = &outb
	cmd.Stderr = &errb
	timer := time.AfterFunc(60*time.Second, func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
	})
	defer timer.Stop()
	runErr := cmd.Run()
	exitCode := 0
	if runErr != nil {
		// Non-zero exit is the expected path here (G0c refuses because doctor
		// is structurally unhealthy in a scratch fixture). Only a spawn failure
		// (not an ExitError) is fatal.
		if exitErr, ok := runErr.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			t.Fatalf("bash spawn error: %v\nstderr: %s", runErr, errb.String())
		}
	}
	return exitCode, outb.String(), errb.String()
}

// TestReleaseTag_G0c_StalePathBinaryIgnored is the Layer 1 behavioral-closure
// crux. It sets up the EXACT defect scenario — a stale decoy binary winning PATH
// resolution — and proves the ceremony no longer halts on it: G0c resolves the
// ceremony-local ./bin/vh-agent-harness (the fresh binary), ignores the decoy,
// and the only refuse that can come from G0c is a legitimate doctor red against
// the FRESH binary (doctor is structurally unhealthy in a scratch fixture).
//
// Outcome observed: the refuse (when it comes) is "doctor not HEALTHY" with real
// fresh-binary doctor output — NOT the old "stale binary on PATH" canary message
// and NOT the decoy's marker. A regression to PATH resolution would surface one
// of those two, failing the assertions below.
func TestReleaseTag_G0c_StalePathBinaryIgnored(t *testing.T) {
	scratch, wrapper, _, _, _ := setupReleaseTagManifestRepo(t, manifestSpecForReadiness())

	// Write a lineage.yml so G0c's seam-installation gate runs doctor (without
	// it, G0c skips and the Layer 1 fix is not exercised).
	linBody := "lineage_version: \"1\"\ntemplate:\n    source: templates/core\n    commit: \"\"\n    ref: test\nrender:\n    rendered_by: test\n"
	commitScratchFile(t, scratch, ".vh-agent-harness/lineage.yml", linBody, "lineage for G0c path test")
	restampManifest(t, scratch, manifestSpecForReadiness())

	// Place the FRESH ceremony binary at ./bin/vh-agent-harness (the path G0c
	// resolves after the Layer 1 pin). The scratch .gitignore already covers
	// bin/, so this stays invisible to G0b.
	copyHarnessBinaryToCeremony(t, scratch)

	// Create a decoy stale binary that WINS PATH resolution.
	decoyDir := t.TempDir()
	writeDecoyStaleBinary(t, decoyDir)

	msgFile := filepath.Join(t.TempDir(), "msg.txt")
	if err := os.WriteFile(msgFile, []byte("release v0.2.0\n\n-test\n"), 0o644); err != nil {
		t.Fatalf("write msg: %v", err)
	}

	exitCode, stdout, stderr := runG0cCeremony(t, wrapper, msgFile, "v0.2.0", decoyDir)

	// The ceremony must REFUSE (doctor is structurally unhealthy in the scratch
	// fixture — a legitimate red). A clean exit 0 here would mean doctor passed
	// against a scratch tree lacking a full .opencode render, which is itself a
	// defect. The point is not "ceremony completes" but "the refuse reason is
	// the fresh-binary doctor red, never the stale-PATH defect".
	if exitCode == 0 {
		t.Fatalf("G0c must REFUSE (doctor unhealthy in scratch); got exit 0\nstdout: %s\nstderr: %s", stdout, stderr)
	}

	// CRUX ASSERTION 1 — the refuse is NOT the old stale-PATH canary message.
	// A regression to PATH resolution would make the canary (`<decoy> release
	// inject-errata --help`) fail and the wrapper would emit the "staleness
	// guard ... on PATH" message.
	if strings.Contains(stdout, "staleness guard") || strings.Contains(stderr, "staleness guard") {
		t.Errorf("REGRESSION: refuse hit the stale-binary canary (PATH was resolved, not ./bin/vh-agent-harness)\nstdout: %s\nstderr: %s", stdout, stderr)
	}
	if strings.Contains(stdout, "binary on PATH") || strings.Contains(stderr, "binary on PATH") {
		t.Errorf("REGRESSION: refuse mentions 'binary on PATH' (defect message present)\nstdout: %s\nstderr: %s", stdout, stderr)
	}

	// CRUX ASSERTION 2 — the refuse IS the fresh-binary doctor red. This proves
	// G0c ran doctor against ./bin/vh-agent-harness, not the decoy.
	if !strings.Contains(stdout, "G0c doctor not HEALTHY") {
		t.Errorf("refuse must be 'G0c doctor not HEALTHY' (fresh binary ran); got stdout:\n%s\nstderr:\n%s", stdout, stderr)
	}

	// CRUX ASSERTION 3 — the decoy was NEVER invoked. Its marker must not appear
	// anywhere (the canary suppressed stderr, but doctor captures 2>&1; if the
	// decoy had been resolved at either stage, the marker would surface).
	if strings.Contains(stdout, decoyBinaryMarker) || strings.Contains(stderr, decoyBinaryMarker) {
		t.Errorf("REGRESSION: decoy stale binary was invoked (PATH resolved); marker found in output\nstdout: %s\nstderr: %s", stdout, stderr)
	}

	// The tag must NOT exist (the ceremony refused).
	if tagExists(t, scratch, "v0.2.0") {
		t.Errorf("tag v0.2.0 must NOT exist after G0c refusal")
	}
}

// TestReleaseTag_G0c_MissingCeremonyBinary_RecipeListRed proves the
// robustness-note path: when the ceremony skipped `make build` (no
// ./bin/vh-agent-harness), G0c refuses with the "ceremony binary missing — run
// `make build` first" message. This is the Layer 2 recipe-list red (AUTO-RECOVER
// recipe #1: make build + retry), NOT a PATH staleness — the decoy on PATH is
// ignored, and the wrapper names the recipe explicitly.
func TestReleaseTag_G0c_MissingCeremonyBinary_RecipeListRed(t *testing.T) {
	scratch, wrapper, _, _, _ := setupReleaseTagManifestRepo(t, manifestSpecForReadiness())

	linBody := "lineage_version: \"1\"\ntemplate:\n    source: templates/core\n    commit: \"\"\n    ref: test\nrender:\n    rendered_by: test\n"
	commitScratchFile(t, scratch, ".vh-agent-harness/lineage.yml", linBody, "lineage for missing-binary test")
	restampManifest(t, scratch, manifestSpecForReadiness())

	// Deliberately do NOT place ./bin/vh-agent-harness — simulate a ceremony
	// that skipped `make build`. Put a decoy on PATH to prove the wrapper does
	// not fall back to PATH resolution.
	decoyDir := t.TempDir()
	writeDecoyStaleBinary(t, decoyDir)

	msgFile := filepath.Join(t.TempDir(), "msg.txt")
	if err := os.WriteFile(msgFile, []byte("release v0.2.0\n\n-test\n"), 0o644); err != nil {
		t.Fatalf("write msg: %v", err)
	}

	exitCode, stdout, stderr := runG0cCeremony(t, wrapper, msgFile, "v0.2.0", decoyDir)

	if exitCode == 0 {
		t.Fatalf("G0c must REFUSE (ceremony binary missing); got exit 0\nstdout: %s\nstderr: %s", stdout, stderr)
	}
	// The refuse must name the ceremony binary and the `make build` recipe — the
	// recipe-list red that seeds Layer 2 AUTO-RECOVER.
	if !strings.Contains(stdout, "ceremony binary missing") {
		t.Errorf("refuse must mention 'ceremony binary missing'; got stdout:\n%s", stdout)
	}
	if !strings.Contains(stdout, "make build") {
		t.Errorf("refuse must name the `make build` recipe; got stdout:\n%s", stdout)
	}
	// The refuse must NOT be the stale-PATH canary (the wrapper did not resolve
	// PATH at all — it refused before the canary, on the missing-binary check).
	if strings.Contains(stdout, "staleness guard") {
		t.Errorf("refuse hit the staleness guard instead of the missing-binary check (unexpected)\nstdout: %s", stdout)
	}
	if strings.Contains(stdout, decoyBinaryMarker) || strings.Contains(stderr, decoyBinaryMarker) {
		t.Errorf("decoy was invoked despite missing ceremony binary (PATH fallback); marker found\nstdout: %s\nstderr: %s", stdout, stderr)
	}
	if tagExists(t, scratch, "v0.2.0") {
		t.Errorf("tag v0.2.0 must NOT exist after missing-binary refusal")
	}
}

// TestReleaseTag_G0c_MissingCeremonyBinary_RecipeRecovers proves the Layer 2
// AUTO-RECOVER recipe end-to-end at the wrapper level: the FIRST run refuses
// with the missing-binary recipe red; applying the recipe (`make build` → place
// ./bin/vh-agent-harness) and retrying ONCE lets G0c proceed past the
// missing-binary check (and then refuse on the legitimate doctor red, exactly
// like TestReleaseTag_G0c_StalePathBinaryIgnored). This is the mechanical proxy
// for "apply recipe, retry once, log" — the retry provably advances past the
// recipe red.
func TestReleaseTag_G0c_MissingCeremonyBinary_RecipeRecovers(t *testing.T) {
	scratch, wrapper, _, _, _ := setupReleaseTagManifestRepo(t, manifestSpecForReadiness())

	linBody := "lineage_version: \"1\"\ntemplate:\n    source: templates/core\n    commit: \"\"\n    ref: test\nrender:\n    rendered_by: test\n"
	commitScratchFile(t, scratch, ".vh-agent-harness/lineage.yml", linBody, "lineage for recipe-recovery test")
	restampManifest(t, scratch, manifestSpecForReadiness())

	decoyDir := t.TempDir()
	writeDecoyStaleBinary(t, decoyDir)

	msgFile := filepath.Join(t.TempDir(), "msg.txt")
	if err := os.WriteFile(msgFile, []byte("release v0.2.0\n\n-test\n"), 0o644); err != nil {
		t.Fatalf("write msg: %v", err)
	}

	// FIRST run: no ceremony binary → recipe-list red.
	exitCode1, stdout1, _ := runG0cCeremony(t, wrapper, msgFile, "v0.2.0", decoyDir)
	if exitCode1 == 0 {
		t.Fatalf("first run must REFUSE (ceremony binary missing); got exit 0\nstdout: %s", stdout1)
	}
	if !strings.Contains(stdout1, "ceremony binary missing") {
		t.Fatalf("first run refuse must name the recipe red; got stdout:\n%s", stdout1)
	}

	// APPLY RECIPE (the Layer 2 auto-recovery step): `make build` produces
	// ./bin/vh-agent-harness. We simulate it by copying the once-built binary.
	copyHarnessBinaryToCeremony(t, scratch)

	// RETRY ONCE: the ceremony now advances PAST the missing-binary check
	// (proving the recipe worked) and reaches doctor, refusing on the
	// legitimate fresh-binary doctor red — identical to the stale-PATH-ignored
	// outcome. No STOP-AND-ASK for the missing-binary red.
	exitCode2, stdout2, stderr2 := runG0cCeremony(t, wrapper, msgFile, "v0.2.0", decoyDir)
	if exitCode2 == 0 {
		t.Fatalf("retry must still REFUSE (doctor unhealthy in scratch); got exit 0\nstdout: %s", stdout2)
	}
	// The retry advanced past the missing-binary check — the refuse is now the
	// doctor red, NOT the recipe red. This is the "recipe applied, gate advanced"
	// signal.
	if strings.Contains(stdout2, "ceremony binary missing") {
		t.Errorf("retry must NOT hit the missing-binary red again (recipe failed to recover);\nstdout: %s", stdout2)
	}
	if !strings.Contains(stdout2, "G0c doctor not HEALTHY") {
		t.Errorf("retry refuse must be the fresh-binary doctor red (recipe recovered);\nstdout: %s\nstderr: %s", stdout2, stderr2)
	}
	if strings.Contains(stdout2, decoyBinaryMarker) || strings.Contains(stderr2, decoyBinaryMarker) {
		t.Errorf("decoy invoked on retry (PATH resolved despite fresh ceremony binary)\nstdout: %s\nstderr: %s", stdout2, stderr2)
	}
}
