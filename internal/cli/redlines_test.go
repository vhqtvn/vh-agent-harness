package cli

// redlines_test.go tests `vh-agent-harness redlines guidance` — the local
// agent-context command. All fixtures use OBVIOUSLY synthetic terms.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// setRedlinesXDG points the user-level redlines registry at a temp dir and
// returns that dir plus a cleanup. Mirrors the redlines package's setXDG.
func setRedlinesXDG(t *testing.T) (string, func()) {
	t.Helper()
	dir := t.TempDir()
	old, had := os.LookupEnv("XDG_CONFIG_HOME")
	os.Setenv("XDG_CONFIG_HOME", dir)
	return dir, func() {
		if had {
			os.Setenv("XDG_CONFIG_HOME", old)
		} else {
			os.Unsetenv("XDG_CONFIG_HOME")
		}
	}
}

// writeRedlinesUserReg writes content as the user-level registry under the XDG
// dir that setRedlinesXDG installed.
func writeRedlinesUserReg(t *testing.T, content string) {
	t.Helper()
	xdg := os.Getenv("XDG_CONFIG_HOME")
	if xdg == "" {
		t.Fatal("test precondition: XDG_CONFIG_HOME not set")
	}
	regDir := filepath.Join(xdg, "vh-agent-harness", "redlines")
	if err := os.MkdirAll(regDir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", regDir, err)
	}
	if err := os.WriteFile(filepath.Join(regDir, "registry.yml"), []byte(content), 0o600); err != nil {
		t.Fatalf("write registry: %v", err)
	}
}

// TestRedlinesGuidance_NoRegistryIsInert is the inert-case test: with no
// user-level registry, guidance exits 0, prints a single short line, emits NO
// terms, and writes NO files anywhere.
func TestRedlinesGuidance_NoRegistryIsInert(t *testing.T) {
	xdg, xdgCleanup := setRedlinesXDG(t)
	defer xdgCleanup()
	repoDir := t.TempDir()

	// Snapshot entry counts to prove zero footprint.
	assertRedlinesNoNewFiles(t, xdg, "XDG dir")
	assertRedlinesNoNewFiles(t, repoDir, "repo dir")

	var out string
	var runErr error
	runWithCwd(t, repoDir, func() {
		out, runErr = executeCapture(t, []string{"redlines", "guidance"})
	})
	if runErr != nil {
		t.Fatalf("inert guidance: want nil error (exit 0), got %v", runErr)
	}
	if !strings.Contains(out, "no registry configured") {
		t.Errorf("inert output should say 'no registry configured'; got:\n%s", out)
	}
	// No terms may appear in the inert output.
	for _, banned := range []string{"synthetic", "subj-", "label", "side_"} {
		if strings.Contains(out, banned) {
			t.Errorf("inert output must not contain %q (leak): %s", banned, out)
		}
	}
}

// TestRedlinesGuidance_NonBindingRegistryIsInert: a registry exists but no
// subject binds this repo. Still exit 0, short line, no terms.
func TestRedlinesGuidance_NonBindingRegistryIsInert(t *testing.T) {
	setRedlinesXDG(t)
	writeRedlinesUserReg(t, `version: 1
subjects:
  - id: subj-test-scrub
    kind: scrub-project
    labels: [synthetic-alpha]
    repos: ["/totally/elsewhere/**"]
`)
	repoDir := t.TempDir()

	var out string
	var runErr error
	runWithCwd(t, repoDir, func() {
		out, runErr = executeCapture(t, []string{"redlines", "guidance"})
	})
	if runErr != nil {
		t.Fatalf("non-binding guidance: want nil error, got %v", runErr)
	}
	if !strings.Contains(out, "no redlines bind this repository") {
		t.Errorf("non-binding output should say so; got:\n%s", out)
	}
	if strings.Contains(out, "synthetic-alpha") {
		t.Errorf("non-binding output must not emit terms; got:\n%s", out)
	}
}

// TestRedlinesGuidance_BindingPrintsFullTerms: a binding subject prints the
// preamble banner + the FULL real term set + the why line. This is the ONE
// surface permitted to emit real terms.
func TestRedlinesGuidance_BindingPrintsFullTerms(t *testing.T) {
	setRedlinesXDG(t)
	writeRedlinesUserReg(t, `version: 1
subjects:
  - id: subj-test-scrub
    kind: scrub-project
    labels: [synthetic-alpha, synthetic-beta]
    policy: scrub-before-commit
    why: synthetic scrub rationale
`)
	repoDir := t.TempDir()

	var out string
	var runErr error
	runWithCwd(t, repoDir, func() {
		out, runErr = executeCapture(t, []string{"redlines", "guidance"})
	})
	if runErr != nil {
		t.Fatalf("binding guidance: want nil error, got %v", runErr)
	}

	// Preamble banner must state the privacy / leak-surface contract.
	for _, want := range []string{
		"LOCAL AGENT CONTEXT",
		"PRIVATE",
		"Do NOT paste",
		"commit gate",
		"subj-*",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("preamble missing %q\n--- output ---\n%s", want, out)
		}
	}
	// Full real terms must appear (this is the point of guidance).
	for _, want := range []string{
		"subj-test-scrub",
		"synthetic-alpha",
		"synthetic-beta",
		"scrub-before-commit",
		"synthetic scrub rationale",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("binding output missing %q\n--- output ---\n%s", want, out)
		}
	}
	// Honesty contract must appear at the bottom.
	if !strings.Contains(out, "lexical and best-effort") {
		t.Errorf("honesty contract missing from output:\n%s", out)
	}
}

// TestRedlinesGuidance_RelationPrintsBothSides: a forbidden-relation subject
// prints SideA + SideB terms and the co-occurrence rule.
func TestRedlinesGuidance_RelationPrintsBothSides(t *testing.T) {
	setRedlinesXDG(t)
	writeRedlinesUserReg(t, `version: 1
subjects:
  - id: subj-test-rel
    kind: forbidden-relation
    side_a: [synthetic-org-gamma]
    side_b: [synthetic-domain-delta]
    unit: file
    why: synthetic relation rationale
`)
	repoDir := t.TempDir()

	var out string
	runWithCwd(t, repoDir, func() {
		out, _ = executeCapture(t, []string{"redlines", "guidance"})
	})
	for _, want := range []string{
		"synthetic-org-gamma",
		"synthetic-domain-delta",
		"Co-occurrence rule",
		"unit: file",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("relation output missing %q\n--- output ---\n%s", want, out)
		}
	}
}

// TestRedlinesGuidance_AmbientRendering: when a subject is ambient for the
// current repo path, the AMBIENT note is shown (SideB terms alone leak).
// We use a path-glob ambient_repos that matches the temp repo dir.
func TestRedlinesGuidance_AmbientRendering(t *testing.T) {
	setRedlinesXDG(t)
	repoDir := t.TempDir()
	writeRedlinesUserReg(t, `version: 1
subjects:
  - id: subj-test-amb
    kind: forbidden-relation
    side_a: [synthetic-ambient-org]
    side_b: [synthetic-ambient-domain]
    ambient_repos: ["`+repoDir+`**"]
    why: synthetic ambient rationale
`)

	var out string
	runWithCwd(t, repoDir, func() {
		out, _ = executeCapture(t, []string{"redlines", "guidance"})
	})
	if !strings.Contains(out, "AMBIENT") {
		t.Errorf("ambient subject should render the AMBIENT note;\n--- output ---\n%s", out)
	}
	if !strings.Contains(out, "side_b terms ALONE leak") {
		t.Errorf("ambient note should state side_b-alone leaks;\n--- output ---\n%s", out)
	}
}

// TestRedlinesGuidance_ZeroFootprintNoFileWrites: guidance must NEVER write a
// file anywhere — not under XDG, not under the repo. Output is stdout only.
// This is the load-bearing anti-materialization invariant (the brief's T3/A2/A5
// rejected generated guidance files).
func TestRedlinesGuidance_ZeroFootprintNoFileWrites(t *testing.T) {
	xdg, xdgCleanup := setRedlinesXDG(t)
	defer xdgCleanup()
	writeRedlinesUserReg(t, `version: 1
subjects:
  - id: subj-test-scrub
    kind: scrub-project
    labels: [synthetic-alpha]
`)
	repoDir := t.TempDir()

	assertRedlinesNoNewFiles(t, xdg, "XDG dir")
	assertRedlinesNoNewFiles(t, repoDir, "repo dir")

	runWithCwd(t, repoDir, func() {
		_, _ = executeCapture(t, []string{"redlines", "guidance"})
	})
}

// TestRedlinesGuidance_InvalidRegistryFailsClosed: a present-but-invalid
// registry produces an opaque error (exit non-zero) and never echoes terms.
func TestRedlinesGuidance_InvalidRegistryFailsClosed(t *testing.T) {
	setRedlinesXDG(t)
	writeRedlinesUserReg(t, `version: 1
subjects:
  - id: not-opaque-id
    kind: scrub-project
    labels: [synthetic-alpha]
`)
	repoDir := t.TempDir()

	var out string
	var runErr error
	runWithCwd(t, repoDir, func() {
		out, runErr = executeCapture(t, []string{"redlines", "guidance"})
	})
	if runErr == nil {
		t.Fatal("invalid registry: want non-nil error (fail-closed), got nil")
	}
	// Error must be opaque: it may name the id and a reason, but not echo the
	// label/term. "synthetic-alpha" must NOT appear in the output.
	if strings.Contains(out, "synthetic-alpha") {
		t.Errorf("fail-closed output must not echo terms; got:\n%s", out)
	}
}

// assertRedlinesNoNewFiles snapshots dir's entry count and fails if it grew.
func assertRedlinesNoNewFiles(t *testing.T, dir, label string) {
	t.Helper()
	want, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("readdir %s (%s): %v", dir, label, err)
	}
	t.Cleanup(func() {
		got, err := os.ReadDir(dir)
		if err != nil {
			t.Fatalf("readdir %s (after): %v", dir, err)
		}
		if len(got) != len(want) {
			var names []string
			for _, e := range got {
				names = append(names, e.Name())
			}
			t.Fatalf("%s: file count changed in %s (before=%d after=%d): %v", label, dir, len(want), len(got), names)
		}
	})
}
