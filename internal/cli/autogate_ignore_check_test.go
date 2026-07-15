package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// writeAutoGateLocalConfig writes a `.local.json` override companion under the
// project-level repo-configs/ dir. The existing writeAutoGateConfig helper only
// handles the committed base files (auto-gate-config.json / auto-gate-llm.json),
// not the `.local.json` variants, so this covers those. basename is the exact
// filename (e.g. "auto-gate-config.local.json").
func writeAutoGateLocalConfig(t *testing.T, target, basename, body string) {
	t.Helper()
	dir := filepath.Join(target, ".opencode", "repo-configs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	if err := os.WriteFile(filepath.Join(dir, basename), []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", basename, err)
	}
}

// gitAddForce stages rel into git's index (makes it tracked) even when rel
// matches a .gitignore rule. The D4/D6 tests simulate the real "tracked despite
// an ignore rule" state (a file committed before the rule was added) — `git add`
// alone refuses an ignored path, so `-f` force-tracks it. Skips if git is
// unavailable; gitInit already guards that.
func gitAddForce(t *testing.T, dir, rel string) {
	t.Helper()
	if err := exec.Command("git", "-C", dir, "add", "-f", rel).Run(); err != nil {
		t.Fatalf("git add -f %s: %v", rel, err)
	}
}

// gitSetGlobalExclude points dir's git at a global excludesFile containing body,
// so a match is attributed to a non-portable source (absolute path). Used by the
// D5 test. Returns the excludes file path.
func gitSetGlobalExclude(t *testing.T, dir, body string) string {
	t.Helper()
	exc := filepath.Join(dir, ".git", "globalexclude")
	if err := os.MkdirAll(filepath.Dir(exc), 0o755); err != nil {
		t.Fatalf("mkdir exclude dir: %v", err)
	}
	if err := os.WriteFile(exc, []byte(body), 0o644); err != nil {
		t.Fatalf("write globalexclude: %v", err)
	}
	if err := exec.Command("git", "-C", dir, "config", "core.excludesfile", exc).Run(); err != nil {
		t.Fatalf("git config core.excludesfile: %v", err)
	}
	return exc
}

// gitSetInfoExclude writes body into dir's per-clone .git/info/exclude, so a match
// is attributed to the non-portable source ".git/info/exclude" (local to this
// clone, not shared via checkout). Used by the D5 info-exclude variant. Unlike
// core.excludesFile this needs no config — git reads info/exclude by default.
func gitSetInfoExclude(t *testing.T, dir, body string) {
	t.Helper()
	exc := filepath.Join(dir, ".git", "info", "exclude")
	if err := os.MkdirAll(filepath.Dir(exc), 0o755); err != nil {
		t.Fatalf("mkdir info dir: %v", err)
	}
	if err := os.WriteFile(exc, []byte(body), 0o644); err != nil {
		t.Fatalf("write info/exclude: %v", err)
	}
}

// TestAutoGateIgnore_SkipWhenUnselectedAndNoFiles: overlay unselected + no config
// files present + not a protected scenario → clean no-op (tierSkip). D1 inertness.
func TestAutoGateIgnore_SkipWhenUnselectedAndNoFiles(t *testing.T) {
	dir := t.TempDir()
	isolateXDG(t)
	gitInit(t, dir)

	r := checkAutoGateGitignored(dir)
	if r.tier != tierSkip {
		t.Fatalf("want SKIP when unselected + no files, got %s: %s", r.tier, r.detail)
	}
}

// TestAutoGateIgnore_PassWhenSelectedAndProtected: overlay selected + .gitignore
// carries both rules + no files on disk → PASS (all never-commit paths portably
// protected). The rules match absent paths (git check-ignore is path-pattern based).
func TestAutoGateIgnore_PassWhenSelectedAndProtected(t *testing.T) {
	dir := t.TempDir()
	isolateXDG(t)
	gitInit(t, dir)
	writeProfileOverlays(t, dir, "auto-classifier-pilot")
	writeGitignore(t, dir, ".opencode/repo-configs/*.local.json\n.opencode/repo-configs/auto-gate-llm.json\n")

	r := checkAutoGateGitignored(dir)
	if r.tier == tierSkip {
		t.Skipf("check unavailable in env: %s", r.detail)
	}
	if r.tier != tierPass {
		t.Fatalf("want PASS when selected + both rules present, got %s: %s", r.tier, r.detail)
	}
}

// TestAutoGateIgnore_WarnReadinessWhenSelectedNoRule: overlay selected, no config
// files, .gitignore WITHOUT the rules → WARN (D2 readiness nudge, not a secret
// incident — nothing is on disk to expose).
func TestAutoGateIgnore_WarnReadinessWhenSelectedNoRule(t *testing.T) {
	dir := t.TempDir()
	isolateXDG(t)
	gitInit(t, dir)
	writeProfileOverlays(t, dir, "auto-classifier-pilot")
	writeGitignore(t, dir, ".opencode/state/\n") // unrelated rule only

	r := checkAutoGateGitignored(dir)
	if r.tier == tierSkip {
		t.Skipf("check unavailable in env: %s", r.detail)
	}
	if r.tier != tierWarn {
		t.Fatalf("want WARN (D2 readiness) when selected + no rule, got %s: %s", r.tier, r.detail)
	}
	if !strings.Contains(r.detail, "D2") {
		t.Errorf("WARN should cite D2 readiness; got %q", r.detail)
	}
}

// TestAutoGateIgnore_FailPresentAndNotIgnored: overlay selected, auto-gate-llm.json
// present (untracked), .gitignore WITHOUT the llm rule → FAIL (D3 active never-commit
// breach: the file is on disk and would be staged on the next git add).
func TestAutoGateIgnore_FailPresentAndNotIgnored(t *testing.T) {
	dir := t.TempDir()
	isolateXDG(t)
	gitInit(t, dir)
	writeProfileOverlays(t, dir, "auto-classifier-pilot")
	writeAutoGateConfig(t, dir, "llm", `{"model": "m"}`)
	writeGitignore(t, dir, ".opencode/state/\n") // no llm rule

	r := checkAutoGateGitignored(dir)
	if r.tier == tierSkip {
		t.Skipf("check unavailable in env: %s", r.detail)
	}
	if r.tier != tierFail {
		t.Fatalf("want FAIL (D3 present + not ignored), got %s: %s", r.tier, r.detail)
	}
	if !strings.Contains(r.detail, "D3") || !strings.Contains(r.detail, "NOT gitignored") {
		t.Errorf("FAIL should cite D3 + not-ignored; got %q", r.detail)
	}
}

// TestAutoGateIgnore_FailTrackedEvenIfIgnored: overlay selected, auto-gate-config.local.json
// tracked by git (staged), .gitignore WITH the *.local.json rule → FAIL (D4: an ignore
// rule does NOT untrack an already-added file).
func TestAutoGateIgnore_FailTrackedEvenIfIgnored(t *testing.T) {
	dir := t.TempDir()
	isolateXDG(t)
	gitInit(t, dir)
	writeProfileOverlays(t, dir, "auto-classifier-pilot")
	writeAutoGateLocalConfig(t, dir, "auto-gate-config.local.json", `{"mode": "live"}`)
	writeGitignore(t, dir, ".opencode/repo-configs/*.local.json\n")
	// Stage the file so it is tracked; the ignore rule matches but does NOT untrack.
	rel := filepath.ToSlash(filepath.Join(".opencode", "repo-configs", "auto-gate-config.local.json"))
	gitAddForce(t, dir, rel)

	r := checkAutoGateGitignored(dir)
	if r.tier == tierSkip {
		t.Skipf("check unavailable in env: %s", r.detail)
	}
	if r.tier != tierFail {
		t.Fatalf("want FAIL (D4 tracked), got %s: %s", r.tier, r.detail)
	}
	if !strings.Contains(r.detail, "D4") || !strings.Contains(r.detail, "TRACKED") {
		t.Errorf("FAIL should cite D4 + tracked; got %q", r.detail)
	}
	if !strings.Contains(r.detail, "git rm --cached") {
		t.Errorf("D4 FAIL should name the remediation (git rm --cached); got %q", r.detail)
	}
}

// TestAutoGateIgnore_WarnNonPortableGlobalExclude: overlay selected, auto-gate-llm.json
// present (untracked), protection ONLY via a global core.excludesFile (absolute path),
// NO repo .gitignore rule → WARN (D5: not shared with teammates/CI).
func TestAutoGateIgnore_WarnNonPortableGlobalExclude(t *testing.T) {
	dir := t.TempDir()
	isolateXDG(t)
	gitInit(t, dir)
	writeProfileOverlays(t, dir, "auto-classifier-pilot")
	writeAutoGateConfig(t, dir, "llm", `{"model": "m"}`)
	// No repo .gitignore rule; instead a global excludes file matches the basename.
	gitSetGlobalExclude(t, dir, "auto-gate-llm.json\n")

	r := checkAutoGateGitignored(dir)
	if r.tier == tierSkip {
		t.Skipf("check unavailable in env: %s", r.detail)
	}
	if r.tier != tierWarn {
		t.Fatalf("want WARN (D5 non-portable global exclude), got %s: %s", r.tier, r.detail)
	}
	if !strings.Contains(r.detail, "D5") || !strings.Contains(r.detail, "non-portable") {
		t.Errorf("WARN should cite D5 + non-portable; got %q", r.detail)
	}
}

// TestAutoGateIgnore_WarnNonPortableInfoExclude: overlay selected, auto-gate-llm.json
// present (untracked), protection ONLY via the per-clone .git/info/exclude (local to
// this clone, NOT shared via checkout), NO repo .gitignore rule → WARN (D5:
// non-portable source). Exercises the portableIgnoreSource ".git/info/exclude"
// branch that the core.excludesFile variant above does not cover.
func TestAutoGateIgnore_WarnNonPortableInfoExclude(t *testing.T) {
	dir := t.TempDir()
	isolateXDG(t)
	gitInit(t, dir)
	writeProfileOverlays(t, dir, "auto-classifier-pilot")
	writeAutoGateConfig(t, dir, "llm", `{"model": "m"}`)
	// No repo .gitignore rule; instead the per-clone info/exclude matches the basename.
	gitSetInfoExclude(t, dir, "auto-gate-llm.json\n")

	r := checkAutoGateGitignored(dir)
	if r.tier == tierSkip {
		t.Skipf("check unavailable in env: %s", r.detail)
	}
	if r.tier != tierWarn {
		t.Fatalf("want WARN (D5 non-portable info/exclude), got %s: %s", r.tier, r.detail)
	}
	if !strings.Contains(r.detail, "D5") || !strings.Contains(r.detail, "non-portable") {
		t.Errorf("WARN should cite D5 + non-portable; got %q", r.detail)
	}
}

// TestAutoGateIgnore_FailTrackedLiteralKeyRotate: overlay selected, auto-gate-llm.json
// tracked by git (staged) AND carrying a non-empty literal apiKey, .gitignore WITH the
// llm rule → FAIL (D6 credential incident: rotate/revoke guidance). Also asserts the
// literal key VALUE is never present in the detail (D6 safety).
func TestAutoGateIgnore_FailTrackedLiteralKeyRotate(t *testing.T) {
	dir := t.TempDir()
	isolateXDG(t)
	gitInit(t, dir)
	writeProfileOverlays(t, dir, "auto-classifier-pilot")
	writeAutoGateConfig(t, dir, "llm", `{"apiKey": "sk-live-secret-xyz", "model": "m"}`)
	writeGitignore(t, dir, ".opencode/repo-configs/auto-gate-llm.json\n")
	rel := filepath.ToSlash(filepath.Join(".opencode", "repo-configs", "auto-gate-llm.json"))
	gitAddForce(t, dir, rel)

	r := checkAutoGateGitignored(dir)
	if r.tier == tierSkip {
		t.Skipf("check unavailable in env: %s", r.detail)
	}
	if r.tier != tierFail {
		t.Fatalf("want FAIL (D6 tracked literal key), got %s: %s", r.tier, r.detail)
	}
	if !strings.Contains(r.detail, "D6") || !strings.Contains(r.detail, "rotate") {
		t.Errorf("FAIL should cite D6 + rotate guidance; got %q", r.detail)
	}
	// D6 safety: the literal key value MUST NOT appear anywhere in the output.
	if strings.Contains(r.detail, "sk-live-secret-xyz") {
		t.Fatalf("D6 VIOLATION: literal apiKey value leaked into detail: %q", r.detail)
	}
}

// TestAutoGateLlmHasLiteralKey: D6 literal-key detector truth table, exercised
// directly. Covers the top-level apiKey AND the live-tiered leaves[].apiKey shape
// (each leaf is a full leaf-config object that may carry its own literal key). The
// function returns a bare bool; the key value never escapes (structural guarantee
// of the bool-only signature, reinforced by the end-to-end leaves D6 test below).
func TestAutoGateLlmHasLiteralKey(t *testing.T) {
	cases := []struct {
		name string
		body string
		want bool
	}{
		{"top-level literal apiKey", `{"apiKey":"sk-x","model":"m"}`, true},
		{"empty top-level apiKey falls to env", `{"apiKey":"","apiKeyEnv":"K","model":"m"}`, false},
		{"apiKeyEnv-only top-level", `{"apiKeyEnv":"K","model":"m"}`, false},
		{"non-string top-level apiKey", `{"apiKey":123,"model":"m"}`, false},
		{"leaf with literal apiKey (live-tiered)", `{"mode":"live-tiered","leaves":[{"apiKey":"sk-leaf","model":"m"}]}`, true},
		{"leaf apiKeyEnv-only (no literal)", `{"mode":"live-tiered","leaves":[{"apiKeyEnv":"K","model":"m"}]}`, false},
		{"one env leaf + one literal leaf", `{"mode":"live-tiered","leaves":[{"apiKeyEnv":"K","model":"m"},{"apiKey":"sk-leaf","model":"m"}]}`, true},
		{"leaves present but no apiKey anywhere", `{"mode":"live-tiered","leaves":[{"model":"m","apiKeyEnv":"K"}]}`, false},
		{"leaves not an array", `{"mode":"live-tiered","leaves":"nope"}`, false},
		{"leaf not an object", `{"mode":"live-tiered","leaves":["nope"]}`, false},
		{"no apiKey, no leaves", `{"mode":"audit"}`, false},
		{"malformed JSON", `{not json`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := filepath.Join(t.TempDir(), "auto-gate-llm.json")
			if err := os.WriteFile(f, []byte(tc.body), 0o644); err != nil {
				t.Fatalf("write fixture: %v", err)
			}
			if got := autoGateLlmHasLiteralKey(f); got != tc.want {
				t.Fatalf("autoGateLlmHasLiteralKey = %v, want %v (body=%s)", got, tc.want, tc.body)
			}
		})
	}
	// Missing file → false (read-error path returns false; D4 tracked-FAIL still applies).
	if autoGateLlmHasLiteralKey(filepath.Join(t.TempDir(), "does-not-exist.json")) {
		t.Fatal("missing file should report false")
	}
}

// TestAutoGateIgnore_FailTrackedLiteralKeyInLeaves: overlay selected, a live-tiered
// auto-gate-llm.json tracked by git AND carrying a literal apiKey INSIDE leaves[],
// .gitignore WITH the llm rule → FAIL (D6: leaves[].apiKey is a literal key on disk
// too, not just the top-level field). Also asserts the literal value never reaches
// the detail (D6 never-emits invariant), mirroring the top-level rotate test.
func TestAutoGateIgnore_FailTrackedLiteralKeyInLeaves(t *testing.T) {
	dir := t.TempDir()
	isolateXDG(t)
	gitInit(t, dir)
	writeProfileOverlays(t, dir, "auto-classifier-pilot")
	writeAutoGateConfig(t, dir, "llm", `{"mode":"live-tiered","leaves":[{"apiKey":"sk-leaf-secret-xyz","model":"m"}]}`)
	writeGitignore(t, dir, ".opencode/repo-configs/auto-gate-llm.json\n")
	rel := filepath.ToSlash(filepath.Join(".opencode", "repo-configs", "auto-gate-llm.json"))
	gitAddForce(t, dir, rel)

	r := checkAutoGateGitignored(dir)
	if r.tier == tierSkip {
		t.Skipf("check unavailable in env: %s", r.detail)
	}
	if r.tier != tierFail {
		t.Fatalf("want FAIL (D6 tracked literal key in leaves), got %s: %s", r.tier, r.detail)
	}
	if !strings.Contains(r.detail, "D6") || !strings.Contains(r.detail, "rotate") {
		t.Errorf("FAIL should cite D6 + rotate guidance; got %q", r.detail)
	}
	// D6 safety: the literal key value MUST NOT appear anywhere in the output.
	if strings.Contains(r.detail, "sk-leaf-secret-xyz") {
		t.Fatalf("D6 VIOLATION: literal leaf apiKey value leaked into detail: %q", r.detail)
	}
}
