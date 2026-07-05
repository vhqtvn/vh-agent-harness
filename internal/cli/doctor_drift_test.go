package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	corpus "github.com/vhqtvn/vh-agent-harness"
	"github.com/vhqtvn/vh-agent-harness/internal/ownership"
)

// writeOwnershipOverrides writes a harness-ownership.yml under
// <target>/.vh-agent-harness/ raising each path->class. This is the S2 authority
// readOwnershipOverrides consumes. Path keys use repo-relative slash form.
func writeOwnershipOverrides(t *testing.T, target string, raises map[string]string) {
	t.Helper()
	dir := filepath.Join(target, ".vh-agent-harness")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	var b strings.Builder
	b.WriteString("overrides:\n")
	for p, c := range raises {
		b.WriteString("  ")
		b.WriteString(p)
		b.WriteString(":\n    class: ")
		b.WriteString(c)
		b.WriteString("\n    reason: \"test raise\"\n")
	}
	if err := os.WriteFile(filepath.Join(dir, ownershipOverridesFileName), []byte(b.String()), 0o644); err != nil {
		t.Fatalf("write ownership overrides: %v", err)
	}
}

// findLivePlatformManagedPath returns the repo-relative slash path of a corpus
// platform_managed path that exists on disk under root. Used to pick a robust
// fixture path that is independent of which files a given profile renders.
func findLivePlatformManagedPath(t *testing.T, root string) string {
	t.Helper()
	def, err := corpus.CoreOwnershipDefaults()
	if err != nil {
		t.Fatalf("core ownership defaults: %v", err)
	}
	for p, rule := range def {
		if rule.Class != ownership.ClassPlatformManaged {
			continue
		}
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(p))); err == nil {
			return p
		}
	}
	t.Fatalf("no live platform_managed path found under %s", root)
	return ""
}

// findLiveDefaultNonManagedPath returns the repo-relative slash path of a corpus
// path whose DEFAULT ownership class is project_owned (e.g. a render-independent
// seed like README.md, Makefile, CLAUDE.md, .gitignore, docs/planning/backlog.md,
// docs/planning/roadmap.md, or forbidden-patterns.project.js) and that exists on
// disk under root. Used to exercise the F3 regression: a default-class non-managed
// file that diverges must be silently skipped — not labeled "project-preserved
// (ownership override)".
//
// Render-independence rationale (why this is restricted to ClassProjectOwned):
// checkManagedDrift's re-render reads SOME non-managed files as inputs. The
// platform_armed vh-harness-profile.yml is consumed by readProfileAnswers, and
// external_generated recon data is consumed by the recon loader. Corrupting
// either of those in the test would corrupt the re-render itself and surface an
// orthogonal drift failure (e.g. opencode.jsonc drifting because a profile seed
// was dropped) unrelated to the F3 assertion. ClassProjectOwned seeds are NOT
// read as render inputs, so corrupting one exercises exactly the F3 path
// (default-class divergence silently skipped) with no render-input coupling.
// platform_managed is excluded because it is the F3 in-scope class, not the
// default-non-managed case under test.
func findLiveDefaultNonManagedPath(t *testing.T, root string) string {
	t.Helper()
	def, err := corpus.CoreOwnershipDefaults()
	if err != nil {
		t.Fatalf("core ownership defaults: %v", err)
	}
	for p, rule := range def {
		if rule.Class != ownership.ClassProjectOwned {
			continue
		}
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(p))); err == nil {
			return p
		}
	}
	t.Fatalf("no live default non-managed path found under %s", root)
	return ""
}

// TestManagedDrift_NoOverride_Pass: a clean seam install with no
// harness-ownership.yml must report managed-drift as PASS with "in sync" detail.
// This is the unchanged-behavior baseline: the override-awareness path must be a
// no-op when no override file is present.
func TestManagedDrift_NoOverride_Pass(t *testing.T) {
	root := t.TempDir()
	seamInstallInto(t, root)

	r := checkManagedDrift(root)
	if r.tier != tierPass {
		t.Fatalf("want PASS for clean install (no overrides), got %s: %s", r.tier, r.detail)
	}
	if !strings.Contains(r.detail, "in sync") {
		t.Errorf("PASS detail should say 'in sync'; got %q", r.detail)
	}
	if strings.Contains(r.detail, "preserved") {
		t.Errorf("PASS detail should not mention preserved when no overrides; got %q", r.detail)
	}
}

// TestManagedDrift_NoOverride_Divergent_StillFails: regression guard. Without an
// override, divergent bytes on a platform_managed path must still FAIL. This
// proves the override-awareness change did not silently disable real drift
// detection for the common (no-override) case.
func TestManagedDrift_NoOverride_Divergent_StillFails(t *testing.T) {
	root := t.TempDir()
	seamInstallInto(t, root)
	p := findLivePlatformManagedPath(t, root)
	live := filepath.Join(root, filepath.FromSlash(p))
	if err := os.WriteFile(live, []byte("// intentionally divergent bytes\n"), 0o644); err != nil {
		t.Fatalf("corrupt %s: %v", p, err)
	}

	r := checkManagedDrift(root)
	if r.tier != tierFail {
		t.Fatalf("want FAIL for divergent managed file with no override, got %s: %s", r.tier, r.detail)
	}
	if !strings.Contains(r.detail, "drifted") {
		t.Errorf("FAIL detail should report drift; got %q", r.detail)
	}
}

// TestManagedDrift_OverrideProjectOwned_Divergent_Preserved: the core A2 fix. A
// platform_managed path raised to project_owned via harness-ownership.yml, with
// divergent live bytes, must report as a NON-FAILING preserved (tierInfo) signal
// — NOT as a perpetual drifted FAIL. update preserves project_owned divergences
// by design (substrate.Apply ActionProjectPreserved); doctor must agree.
func TestManagedDrift_OverrideProjectOwned_Divergent_Preserved(t *testing.T) {
	root := t.TempDir()
	seamInstallInto(t, root)
	p := findLivePlatformManagedPath(t, root)

	// Diverge the live bytes (this is what would have been drift before A2).
	live := filepath.Join(root, filepath.FromSlash(p))
	if err := os.WriteFile(live, []byte("// hand-curated project content; must be preserved\n"), 0o644); err != nil {
		t.Fatalf("diverge %s: %v", p, err)
	}
	// Raise the path to project_owned via the S2 override authority.
	writeOwnershipOverrides(t, root, map[string]string{p: string(ownership.ClassProjectOwned)})

	r := checkManagedDrift(root)
	if r.tier != tierInfo {
		t.Fatalf("want INFO (preserved) for overridden+divergent project_owned path, got %s: %s", r.tier, r.detail)
	}
	if !strings.Contains(r.detail, "preserved") {
		t.Errorf("INFO detail should mention preserved; got %q", r.detail)
	}
}

// TestManagedDrift_DefaultNonManaged_Divergent_NotPreserved is the F3 regression
// guard. A path whose DEFAULT ownership class is project_owned (a render-
// independent seed like README.md, Makefile, CLAUDE.md, .gitignore,
// docs/planning/backlog.md, docs/planning/roadmap.md, or
// forbidden-patterns.project.js) that diverges from the render must be SILENTLY
// SKIPPED — NOT counted as `preserved` and NOT labeled "project-preserved
// (ownership override)". These diverge by design (operator-curated) and are NOT
// ownership overrides. Only a genuine override-raise (Origin == OriginOverrideRaise)
// may surface as preserved.
//
// Candidate-set note: the helper deliberately restricts to ClassProjectOwned.
// platform_armed (vh-harness-profile.yml) and external_generated (recon data)
// are NOT safe candidates here — the re-render inside checkManagedDrift reads
// them as inputs (readProfileAnswers / recon loader), so corrupting them would
// corrupt the re-render and trip an orthogonal drift failure unrelated to the
// F3 assertion. ClassProjectOwned seeds are never read as render inputs.
//
// This is the exact gap that let the A2 over-broadening (F3) slip: the preserved
// branch fired for ANY effective class != platform_managed, mislabeling the 8
// default-class files in this dogfood repo on every install. The fix narrows the
// gate to origin == override-raise; this test pins the narrowed behavior.
func TestManagedDrift_DefaultNonManaged_Divergent_NotPreserved(t *testing.T) {
	root := t.TempDir()
	seamInstallInto(t, root)
	p := findLiveDefaultNonManagedPath(t, root)
	// Diverge the live bytes — this is the condition that, pre-fix, was mislabeled
	// "project-preserved (ownership override)".
	live := filepath.Join(root, filepath.FromSlash(p))
	if err := os.WriteFile(live, []byte("// intentionally divergent default-class bytes\n"), 0o644); err != nil {
		t.Fatalf("corrupt %s: %v", p, err)
	}
	// NO harness-ownership.yml — this is a default-class divergence, not an override.

	r := checkManagedDrift(root)
	if r.tier != tierPass {
		t.Fatalf("want PASS (default-class divergence is silent, not preserved) for %s (default class on this corpus), got %s: %s",
			p, r.tier, r.detail)
	}
	if strings.Contains(r.detail, "preserved") {
		t.Errorf("PASS detail must not mention preserved for a default-class divergence; got %q", r.detail)
	}
	if !strings.Contains(r.detail, "in sync") {
		t.Errorf("PASS detail should say 'in sync'; got %q", r.detail)
	}
}

// TestManagedDrift_OverrideProjectOwned_MissingFile_NotPreserved: an overridden
// project_owned path that is MISSING from disk must NOT be counted as preserved
// (a different condition) and must NOT be counted as missing/drifted. A raised
// path is the operator's concern — update never seeds or touches it — so its
// absence is silent. This guards against conflating preserved with missing.
func TestManagedDrift_OverrideProjectOwned_MissingFile_NotPreserved(t *testing.T) {
	root := t.TempDir()
	seamInstallInto(t, root)
	p := findLivePlatformManagedPath(t, root)

	// Remove the live file entirely, then raise it to project_owned.
	live := filepath.Join(root, filepath.FromSlash(p))
	if err := os.Remove(live); err != nil {
		t.Fatalf("remove %s: %v", p, err)
	}
	writeOwnershipOverrides(t, root, map[string]string{p: string(ownership.ClassProjectOwned)})

	r := checkManagedDrift(root)
	// No drift, no missing (the path is no longer platform_managed-effective),
	// no preserved divergence (file is absent). Outcome is a clean PASS with no
	// "preserved" mention.
	if r.tier != tierPass {
		t.Fatalf("want PASS (missing project_owned path is silent), got %s: %s", r.tier, r.detail)
	}
	if strings.Contains(r.detail, "preserved") {
		t.Errorf("missing raised path must not be reported as preserved; got %q", r.detail)
	}
	if strings.Contains(r.detail, "missing") {
		t.Errorf("missing raised path must not be reported as missing; got %q", r.detail)
	}
}

// TestManagedDrift_InvalidOverride_FailsClean: a present-but-invalid ownership
// override (unknown class literal) must FAIL cleanly rather than silently
// honoring or ignoring the amendment. Validation happens in one of two layers —
// readOwnershipOverrides rejects unknown class literals early, ownership.Resolve
// rejects downgrades / unknown paths — and doctor surfaces whichever fires so
// the operator fixes the override file.
func TestManagedDrift_InvalidOverride_FailsClean(t *testing.T) {
	root := t.TempDir()
	seamInstallInto(t, root)
	p := findLivePlatformManagedPath(t, root)
	writeOwnershipOverrides(t, root, map[string]string{p: "not-a-real-class"})

	r := checkManagedDrift(root)
	if r.tier != tierFail {
		t.Fatalf("want FAIL for invalid override class, got %s: %s", r.tier, r.detail)
	}
	if !strings.Contains(r.detail, "ownership") {
		t.Errorf("FAIL detail should name the ownership validation error; got %q", r.detail)
	}
	if !strings.Contains(r.detail, "not-a-real-class") {
		t.Errorf("FAIL detail should name the offending class; got %q", r.detail)
	}
}

// TestPreflight_PreservedIsNonBlocking: end-to-end via the preflight entry path.
// An overridden+divergent project_owned path surfaces as INFO (preserved) and
// preflight must treat it as PASS — never blocking install/update on a preserved
// file. Verifies the shared checkManagedDrift fix flows through preflight's
// tier-handling correctly (failed count stays 0 -> exit 0).
func TestPreflight_PreservedIsNonBlocking(t *testing.T) {
	root := t.TempDir()
	seamInstallInto(t, root)
	p := findLivePlatformManagedPath(t, root)

	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(p)),
		[]byte("// hand-curated; ownership-raised\n"), 0o644); err != nil {
		t.Fatalf("diverge %s: %v", p, err)
	}
	writeOwnershipOverrides(t, root, map[string]string{p: string(ownership.ClassProjectOwned)})

	var out string
	var runErr error
	runWithCwd(t, root, func() {
		cmd, buf := newOutCmd()
		runErr = runPreflight(cmd, []string{})
		out = buf.String()
	})

	if runErr != nil {
		t.Fatalf("preflight must PASS (non-blocking) on a preserved file; got err=%v out=%q", runErr, out)
	}
	if !strings.Contains(out, "result: PASS") {
		t.Fatalf("preflight output should report PASS; got:\n%s", out)
	}
	// The managed-drift row should carry the INFO tier + preserved detail, proving
	// the signal is surfaced (not silently swallowed) while still non-blocking.
	if !strings.Contains(out, "INFO") || !strings.Contains(out, "preserved") {
		t.Errorf("preflight should surface the INFO/preserved managed-drift row; got:\n%s", out)
	}
}
