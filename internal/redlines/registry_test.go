package redlines

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// All registry content in this file is OBVIOUSLY synthetic. No real entry is
// represented. Registry fixtures are written into per-test temp directories so
// NO committed fixture file could be mistaken for a real registry.

// setXDG redirects the user-level registry into a temp dir and returns that
// dir plus a cleanup that restores the prior env. The registry lives at
// <dir>/vh-agent-harness/redlines/registry.yml.
func setXDG(t *testing.T) (string, func()) {
	t.Helper()
	dir := t.TempDir()
	old, hadOld := os.LookupEnv("XDG_CONFIG_HOME")
	os.Setenv("XDG_CONFIG_HOME", dir)
	return dir, func() {
		if hadOld {
			os.Setenv("XDG_CONFIG_HOME", old)
		} else {
			os.Unsetenv("XDG_CONFIG_HOME")
		}
	}
}

// writeUserReg writes the given content as the user-level registry under the
// XDG dir that setXDG installed.
func writeUserReg(t *testing.T, content string) {
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

// writeRepoLocal writes content as the repo-local additive registry under
// repoRoot/.vh-agent-harness/redlines.local.yml.
func writeRepoLocal(t *testing.T, repoRoot, content string) {
	t.Helper()
	dir := filepath.Join(repoRoot, ".vh-agent-harness")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "redlines.local.yml"), []byte(content), 0o600); err != nil {
		t.Fatalf("write repo-local: %v", err)
	}
}

// assertNoNewFiles snapshots dir's entry count and fails if it grew. Used to
// prove the zero-footprint property across a Load call.
func assertNoNewFiles(t *testing.T, dir, label string) {
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

const validScrub = `version: 1
subjects:
  - id: subj-test-scrub
    kind: scrub-project
    labels: [synthetic-alpha, synthetic-beta]
    policy: scrub-before-commit
    why: synthetic private rationale
`

const validRelation = `version: 1
subjects:
  - id: subj-test-relation
    kind: forbidden-relation
    side_a: [synthetic-gamma]
    side_b: [synthetic-delta]
    ambient_repos: ["github.com/synthetic-org/*"]
    unit: file
    why: synthetic private rationale
`

func TestLoad_NoRegistry_IsInertZeroFootprint(t *testing.T) {
	// The SINGLE most important property: with no user-level registry, Load is
	// a complete no-op — nil registry, nil error, and NOTHING written. This is
	// the case that protects adopters who do not use the feature.
	xdg, cleanup := setXDG(t)
	defer cleanup()
	repoRoot := t.TempDir()

	assertNoNewFiles(t, xdg, "XDG dir")
	assertNoNewFiles(t, repoRoot, "repo root")

	reg, err := Load(repoRoot)
	if err != nil {
		t.Fatalf("Load with no registry: want nil error, got %v", err)
	}
	if reg != nil {
		t.Fatalf("Load with no registry: want nil registry, got %+v", reg)
	}
}

func TestLoad_NoXDGFallbackToHome(t *testing.T) {
	// When XDG_CONFIG_HOME is UNSET, resolution falls back to $HOME/.config.
	// We point HOME at a temp dir and confirm a registry under
	// .config/vh-agent-harness/redlines/registry.yml is found.
	oldXDG, hadXDG := os.LookupEnv("XDG_CONFIG_HOME")
	os.Unsetenv("XDG_CONFIG_HOME")
	defer func() {
		if hadXDG {
			os.Setenv("XDG_CONFIG_HOME", oldXDG)
		}
	}()
	home := t.TempDir()
	oldHome, hadHome := os.LookupEnv("HOME")
	os.Setenv("HOME", home)
	defer func() {
		if hadHome {
			os.Setenv("HOME", oldHome)
		} else {
			os.Unsetenv("HOME")
		}
	}()

	regDir := filepath.Join(home, ".config", "vh-agent-harness", "redlines")
	if err := os.MkdirAll(regDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(regDir, "registry.yml"), []byte(validScrub), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	reg, err := Load(t.TempDir())
	if err != nil {
		t.Fatalf("Load via HOME fallback: %v", err)
	}
	if reg == nil || len(reg.Subjects) != 1 {
		t.Fatalf("Load via HOME fallback: want 1 subject, got %+v", reg)
	}
	if !strings.HasSuffix(reg.SourcePath, filepath.Join(".config", "vh-agent-harness", "redlines", "registry.yml")) {
		t.Errorf("SourcePath not HOME-based: %s", reg.SourcePath)
	}
}

func TestLoad_RelativeXDGIsIgnored(t *testing.T) {
	// A relative XDG_CONFIG_HOME is invalid per spec and must be ignored
	// (fall back to HOME). This prevents a relative value from anchoring the
	// registry under an unexpected working directory.
	home := t.TempDir()
	oldHome, hadHome := os.LookupEnv("HOME")
	os.Setenv("HOME", home)
	defer func() {
		if hadHome {
			os.Setenv("HOME", oldHome)
		} else {
			os.Unsetenv("HOME")
		}
	}()
	oldXDG, hadXDG := os.LookupEnv("XDG_CONFIG_HOME")
	os.Setenv("XDG_CONFIG_HOME", "relative/path")
	defer func() {
		if hadXDG {
			os.Setenv("XDG_CONFIG_HOME", oldXDG)
		} else {
			os.Unsetenv("XDG_CONFIG_HOME")
		}
	}()

	path, err := UserRegistryPath()
	if err != nil {
		t.Fatalf("UserRegistryPath: %v", err)
	}
	if !strings.HasPrefix(path, home) {
		t.Errorf("relative XDG should fall back to HOME; got %s", path)
	}
}

func TestLoad_ValidBindingRegistry(t *testing.T) {
	setXDG(t)
	writeUserReg(t, validScrub)
	repoRoot := t.TempDir()

	reg, err := Load(repoRoot)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if reg == nil || len(reg.Subjects) != 1 {
		t.Fatalf("want 1 subject, got %+v", reg)
	}
	s := reg.Subjects[0]
	if s.ID != "subj-test-scrub" || s.Kind != KindScrubProject {
		t.Errorf("unexpected subject: %+v", s)
	}
	if len(s.Labels) != 2 {
		t.Errorf("labels: %v", s.Labels)
	}
	if s.Policy != "scrub-before-commit" {
		t.Errorf("policy: %q", s.Policy)
	}
	// Binds all (no repos:).
	if !s.Binds(repoRoot, nil) {
		t.Error("subject with no repos should bind all")
	}
}

func TestLoad_ValidNonBindingRegistry(t *testing.T) {
	setXDG(t)
	// A subject scoped to a path that does NOT match this repo.
	nonBinding := `version: 1
subjects:
  - id: subj-test-scrub
    kind: scrub-project
    labels: [synthetic-alpha]
    repos: ["/totally/elsewhere/**"]
`
	writeUserReg(t, nonBinding)
	repoRoot := t.TempDir()

	reg, err := Load(repoRoot)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if reg == nil {
		t.Fatal("non-binding registry should still load (subjects exist)")
	}
	if len(reg.Subjects) != 1 {
		t.Fatalf("want 1 subject, got %d", len(reg.Subjects))
	}
	if reg.Subjects[0].Binds(repoRoot, nil) {
		t.Error("subject should not bind this repo")
	}
}

func TestLoad_RelationDefaultsUnitBoth(t *testing.T) {
	setXDG(t)
	// Omit unit -> default "both" (empty string). Validate accepts it.
	noUnit := `version: 1
subjects:
  - id: subj-test-relation
    kind: forbidden-relation
    side_a: [synthetic-alpha]
    side_b: [synthetic-beta]
`
	writeUserReg(t, noUnit)
	reg, err := Load(t.TempDir())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if reg == nil || reg.Subjects[0].Unit != "" {
		t.Errorf("expected empty Unit (=both), got %+v", reg)
	}
}

func TestLoad_UnitDiffRejected(t *testing.T) {
	// unit: diff is NOT yet implemented in v1 (the engine scans file-level
	// only). The registry MUST fail-closed at load time so an operator cannot
	// silently get weaker file-level protection by configuring unit: diff.
	setXDG(t)
	diffUnit := `version: 1
subjects:
  - id: subj-test-diff
    kind: forbidden-relation
    side_a: [synthetic-alpha]
    side_b: [synthetic-beta]
    unit: diff
`
	writeUserReg(t, diffUnit)
	_, err := Load(t.TempDir())
	if err == nil {
		t.Fatal("expected error for unit: diff (v1 fail-closed), got nil")
	}
	if !strings.Contains(err.Error(), "diff") {
		t.Errorf("error should name the rejected unit value; got: %v", err)
	}
	if !strings.Contains(err.Error(), "subj-test-diff") {
		t.Errorf("error should name the opaque subject id; got: %v", err)
	}
}

func TestLoad_SourceReposRejected(t *testing.T) {
	// source_repos is NOT yet implemented in v1 (the engine's scrubUnitMatches
	// only consults Labels, so source_repos would silently protect nothing).
	// The registry MUST fail-closed at load time so an operator cannot rely on
	// a field the engine never matches — the same honesty contract as unit: diff.
	setXDG(t)
	srcRepos := `version: 1
subjects:
  - id: subj-test-source
    kind: scrub-project
    labels: [synthetic-alpha]
    source_repos: ["github.com/acme/secret"]
`
	writeUserReg(t, srcRepos)
	_, err := Load(t.TempDir())
	if err == nil {
		t.Fatal("expected error for source_repos (v1 fail-closed), got nil")
	}
	if !strings.Contains(err.Error(), "source_repos") {
		t.Errorf("error should name the rejected field; got: %v", err)
	}
	if !strings.Contains(err.Error(), "subj-test-source") {
		t.Errorf("error should name the opaque subject id; got: %v", err)
	}
}

func TestLoad_EmptyOrWhitespaceTermsRejected(t *testing.T) {
	// A term slice that is non-empty by LENGTH but contains an empty or
	// whitespace-only element loads as a seemingly-valid subject that NEVER
	// FIRES: the scanner's scrubUnitMatches / anyTermInContent both `continue`
	// on "" (they skip empty terms rather than matching), so the subject
	// silently protects nothing — letting otherwise-blocked material into the
	// acquired tree. This is the same honesty-contract failure mode the
	// feature already rejects for unit: diff and source_repos, and the registry
	// MUST fail-closed at load time here too. Mirrors TestLoad_UnitDiffRejected
	// and TestLoad_SourceReposRejected.
	setXDG(t)
	cases := []struct {
		name    string
		content string
		field   string // the field name the error must name (labels/side_a/side_b)
		id      string // the opaque id the error must name
	}{
		{
			name: "labels single empty on scrub-project",
			content: `version: 1
subjects:
  - id: subj-test-empty-label
    kind: scrub-project
    labels: [""]
`,
			field: "labels",
			id:    "subj-test-empty-label",
		},
		{
			name: "labels mixed empty and valid on scrub-project",
			content: `version: 1
subjects:
  - id: subj-test-mixed-label
    kind: scrub-project
    labels: [synthetic-alpha, ""]
`,
			field: "labels",
			id:    "subj-test-mixed-label",
		},
		{
			name: "labels whitespace-only on scrub-project",
			content: `version: 1
subjects:
  - id: subj-test-ws-label
    kind: scrub-project
    labels: ["   "]
`,
			field: "labels",
			id:    "subj-test-ws-label",
		},
		{
			name: "side_a single empty on forbidden-relation",
			content: `version: 1
subjects:
  - id: subj-test-empty-sidea
    kind: forbidden-relation
    side_a: [""]
    side_b: [synthetic-beta]
`,
			field: "side_a",
			id:    "subj-test-empty-sidea",
		},
		{
			name: "side_b single empty on forbidden-relation",
			content: `version: 1
subjects:
  - id: subj-test-empty-sideb
    kind: forbidden-relation
    side_a: [synthetic-alpha]
    side_b: [""]
`,
			field: "side_b",
			id:    "subj-test-empty-sideb",
		},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			writeUserReg(t, c.content)
			_, err := Load(t.TempDir())
			if err == nil {
				t.Fatalf("expected error for %s (v1 fail-closed on empty/whitespace term), got nil", c.name)
			}
			if !strings.Contains(err.Error(), c.field) {
				t.Errorf("error should name the rejected field %q; got: %v", c.field, err)
			}
			if !strings.Contains(err.Error(), c.id) {
				t.Errorf("error should name the opaque subject id %q; got: %v", c.id, err)
			}
		})
	}
}

func TestLoad_EmptyOrWhitespaceGlobsRejected(t *testing.T) {
	// A glob slice (repos / ambient_repos) that is non-empty by LENGTH but
	// contains an empty or whitespace-only element loads as a seemingly-valid
	// subject whose binding scope NEVER MATCHES: matchGlob("", name) returns
	// false, so the subject silently protects zero repos (repos) or never
	// applies the ambient scoping (ambient_repos). This is the same
	// honesty-contract failure mode the feature already rejects for unit:
	// diff, source_repos, and empty terms in labels/side_a/side_b. The
	// registry MUST fail-closed at load time here too. Mirrors
	// TestLoad_EmptyOrWhitespaceTermsRejected.
	setXDG(t)
	cases := []struct {
		name    string
		content string
		field   string // the field name the error must name (repos/ambient_repos)
		id      string // the opaque id the error must name
	}{
		{
			name: "repos single empty on scrub-project",
			content: `version: 1
subjects:
  - id: subj-test-empty-repos
    kind: scrub-project
    labels: [synthetic-alpha]
    repos: [""]
`,
			field: "repos",
			id:    "subj-test-empty-repos",
		},
		{
			name: "repos whitespace-only on scrub-project",
			content: `version: 1
subjects:
  - id: subj-test-ws-repos
    kind: scrub-project
    labels: [synthetic-alpha]
    repos: ["   "]
`,
			field: "repos",
			id:    "subj-test-ws-repos",
		},
		{
			name: "repos single empty on forbidden-relation",
			content: `version: 1
subjects:
  - id: subj-test-empty-repos-rel
    kind: forbidden-relation
    side_a: [synthetic-alpha]
    side_b: [synthetic-beta]
    repos: [""]
`,
			field: "repos",
			id:    "subj-test-empty-repos-rel",
		},
		{
			name: "ambient_repos single empty on forbidden-relation",
			content: `version: 1
subjects:
  - id: subj-test-empty-ambient
    kind: forbidden-relation
    side_a: [synthetic-alpha]
    side_b: [synthetic-beta]
    ambient_repos: [""]
`,
			field: "ambient_repos",
			id:    "subj-test-empty-ambient",
		},
		{
			name: "ambient_repos whitespace-only on forbidden-relation",
			content: `version: 1
subjects:
  - id: subj-test-ws-ambient
    kind: forbidden-relation
    side_a: [synthetic-alpha]
    side_b: [synthetic-beta]
    ambient_repos: ["  "]
`,
			field: "ambient_repos",
			id:    "subj-test-ws-ambient",
		},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			writeUserReg(t, c.content)
			_, err := Load(t.TempDir())
			if err == nil {
				t.Fatalf("expected error for %s (v1 fail-closed on empty/whitespace glob), got nil", c.name)
			}
			if !strings.Contains(err.Error(), c.field) {
				t.Errorf("error should name the rejected field %q; got: %v", c.field, err)
			}
			if !strings.Contains(err.Error(), c.id) {
				t.Errorf("error should name the opaque subject id %q; got: %v", c.id, err)
			}
		})
	}
}

func TestLoad_MalformedSchema_FailsClosed(t *testing.T) {
	setXDG(t)
	cases := []struct {
		name    string
		content string
	}{
		{"unsupported version", "version: 2\nsubjects: []\n"},
		{"bad yaml", "version: 1\nsubjects: [this is not: valid: yaml\n"},
		{"kind invalid", "version: 1\nsubjects:\n  - id: subj-test-x\n    kind: bogus-kind\n    labels: [a]\n"},
		{"missing labels scrub", "version: 1\nsubjects:\n  - id: subj-test-x\n    kind: scrub-project\n"},
		{"missing side_a relation", "version: 1\nsubjects:\n  - id: subj-test-x\n    kind: forbidden-relation\n    side_b: [a]\n"},
		{"missing side_b relation", "version: 1\nsubjects:\n  - id: subj-test-x\n    kind: forbidden-relation\n    side_a: [a]\n"},
		{"bad policy", "version: 1\nsubjects:\n  - id: subj-test-x\n    kind: scrub-project\n    labels: [a]\n    policy: allow-all\n"},
		{"bad unit", "version: 1\nsubjects:\n  - id: subj-test-x\n    kind: forbidden-relation\n    side_a: [a]\n    side_b: [b]\n    unit: paragraph\n"},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			writeUserReg(t, c.content)
			_, err := Load(t.TempDir())
			if err == nil {
				t.Fatalf("expected error for %s", c.name)
			}
		})
	}
}

func TestLoad_BadOpaqueID_FailsClosed(t *testing.T) {
	setXDG(t)
	cases := []struct {
		name string
		id   string
	}{
		{"no prefix", "realsubject"},
		{"empty suffix", "subj-"},
		{"has space", "subj-real thing"},
		{"has slash", "subj/path"},
		{"bare subj", "subj"},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			content := "version: 1\nsubjects:\n  - id: " + c.id + "\n    kind: scrub-project\n    labels: [synthetic-alpha]\n"
			writeUserReg(t, content)
			_, err := Load(t.TempDir())
			if err == nil {
				t.Fatalf("expected error for id %q", c.id)
			}
		})
	}
}

func TestLoad_DuplicateIDs_FailsClosed(t *testing.T) {
	setXDG(t)
	dup := `version: 1
subjects:
  - id: subj-test-scrub
    kind: scrub-project
    labels: [synthetic-alpha]
  - id: subj-test-scrub
    kind: scrub-project
    labels: [synthetic-beta]
`
	writeUserReg(t, dup)
	_, err := Load(t.TempDir())
	if err == nil {
		t.Fatal("expected duplicate-id error")
	}
	if !strings.Contains(err.Error(), "subj-test-scrub") {
		t.Errorf("error should name the opaque id; got: %v", err)
	}
}

// requireNonRoot skips the test when running as root (euid 0): root's
// DAC_OVERRIDE ignores file permission bits, so the chmod-0000
// unreadable-registry injection cannot make the load's ReadFile fail
// (the registry reads fine and the fail-closed assertion mis-fires).
// Same canonical helper shape as internal/session, internal/renderstate,
// internal/cli, and internal/substrate (package-local on purpose: Go test
// packages cannot share helpers without an import).
func requireNonRoot(t *testing.T) {
	t.Helper()
	if os.Geteuid() == 0 {
		t.Skip("running as root: DAC_OVERRIDE bypasses permission bits; cannot inject unreadable-registry failure")
	}
}

func TestLoad_UnreadableRegistry_FailsClosed(t *testing.T) {
	requireNonRoot(t)
	if !modesAreMeaningful() {
		t.Skip("platform without POSIX modes; cannot make file unreadable")
	}
	setXDG(t)
	writeUserReg(t, validScrub)
	// Strip all permissions from the registry file so ReadFile fails.
	xdg := os.Getenv("XDG_CONFIG_HOME")
	path := filepath.Join(xdg, "vh-agent-harness", "redlines", "registry.yml")
	if err := os.Chmod(path, 0o000); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0o600) })

	_, err := Load(t.TempDir())
	if err == nil {
		t.Fatal("expected unreadable error")
	}
}

func TestLoad_RepoLocalAdditive_NewSubject(t *testing.T) {
	// A repo-local file may ADD a NEW subject (by a new id). This is the only
	// allowed additive operation.
	setXDG(t)
	writeUserReg(t, validScrub)
	repoRoot := t.TempDir()
	writeRepoLocal(t, repoRoot, `version: 1
subjects:
  - id: subj-local-extra
    kind: scrub-project
    labels: [synthetic-local]
`)

	reg, err := Load(repoRoot)
	if err != nil {
		t.Fatalf("Load with additive repo-local: %v", err)
	}
	if reg == nil || len(reg.Subjects) != 2 {
		t.Fatalf("want 2 subjects (user+local), got %+v", reg)
	}
	ids := map[string]bool{}
	for _, s := range reg.Subjects {
		ids[s.ID] = true
	}
	if !ids["subj-test-scrub"] || !ids["subj-local-extra"] {
		t.Errorf("missing expected ids; got %v", ids)
	}
}

func TestLoad_RepoLocalWeakeningCollides_FailsClosed(t *testing.T) {
	// A repo-local file that redefines a USER-LEVEL id is rejected. This is the
	// additive/tightening-only rule: repo-local can never mask or weaken a
	// user-level entry.
	setXDG(t)
	writeUserReg(t, validScrub)
	repoRoot := t.TempDir()
	writeRepoLocal(t, repoRoot, `version: 1
subjects:
  - id: subj-test-scrub
    kind: scrub-project
    labels: [fewer-labels]
`)

	_, err := Load(repoRoot)
	if err == nil {
		t.Fatal("expected collision error when repo-local redefines a user id")
	}
	if !strings.Contains(err.Error(), "subj-test-scrub") {
		t.Errorf("error should name the colliding opaque id; got: %v", err)
	}
}

func TestLoad_RepoLocalAloneIsNotEnough(t *testing.T) {
	// With NO user-level registry, a repo-local file alone does NOT activate
	// the capability. (Repo-local is additive to a user registry; there is
	// nothing to add to.) This preserves the zero-footprint property for
	// adopters with no user registry even if they happen to have a stray
	// repo-local file.
	setXDG(t)
	repoRoot := t.TempDir()
	writeRepoLocal(t, repoRoot, validScrub)

	reg, err := Load(repoRoot)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if reg != nil {
		t.Fatalf("repo-local alone must be inert; got %+v", reg)
	}
}

func TestLoad_ZeroFootprintInEveryCase(t *testing.T) {
	// Cross-cutting: regardless of outcome (success or fail-closed), Load must
	// NEVER create a file under the XDG dir, the repo root, or the repo's
	// .vh-agent-harness dir.
	xdg, cleanup := setXDG(t)
	defer cleanup()
	repoRoot := t.TempDir()

	writeUserReg(t, validScrub)
	// Snapshot after the user registry is written; Load must not add anything.
	assertNoNewFiles(t, xdg, "XDG dir across Load")
	assertNoNewFiles(t, repoRoot, "repo root across Load")

	if _, err := Load(repoRoot); err != nil {
		t.Fatalf("Load: %v", err)
	}
}

func TestLoad_OpaqueErrorsNeverLeakTerms(t *testing.T) {
	// Every error string must be opaque: it may name a path and a subj-* id,
	// but NEVER a label, termset, remote, or `why` value.
	setXDG(t)
	secret := `version: 1
subjects:
  - id: subj-test-scrub
    kind: scrub-project
    labels: [synthetic-alpha]
    why: this must never appear in an error
`
	writeUserReg(t, secret)
	// Force a duplicate to produce an error that mentions the subject.
	dup := `version: 1
subjects:
  - id: subj-test-scrub
    kind: scrub-project
    labels: [synthetic-alpha]
  - id: subj-test-scrub
    kind: scrub-project
    labels: [synthetic-alpha]
`
	writeUserReg(t, dup)
	_, err := Load(t.TempDir())
	if err == nil {
		t.Fatal("expected error")
	}
	msg := err.Error()
	for _, banned := range []string{"this must never appear in an error", "synthetic-alpha"} {
		if strings.Contains(msg, banned) {
			t.Errorf("error leaked sensitive content %q: %v", banned, err)
		}
	}
}

func TestCheckFileSecurity(t *testing.T) {
	if !modesAreMeaningful() {
		t.Skip("platform without POSIX modes; CheckFileSecurity is a documented no-op here")
	}
	dir := t.TempDir()

	// Missing file: not checked.
	res := CheckFileSecurity(filepath.Join(dir, "absent"))
	if res.Checked {
		t.Errorf("missing file should not be checked: %+v", res)
	}

	// 0600: clean.
	clean := filepath.Join(dir, "clean.yml")
	if err := os.WriteFile(clean, []byte("x"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	res = CheckFileSecurity(clean)
	if !res.Checked || res.GroupOrWorldReadable {
		t.Errorf("0600 file should be clean: %+v", res)
	}

	// 0644: group/world readable -> warn condition.
	open := filepath.Join(dir, "open.yml")
	if err := os.WriteFile(open, []byte("x"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	res = CheckFileSecurity(open)
	if !res.Checked || !res.GroupOrWorldReadable {
		t.Errorf("0644 file should be flagged: %+v", res)
	}

	// 0660: group-only -> still flagged (any group/other access).
	group := filepath.Join(dir, "group.yml")
	if err := os.WriteFile(group, []byte("x"), 0o660); err != nil {
		t.Fatalf("write: %v", err)
	}
	res = CheckFileSecurity(group)
	if !res.Checked || !res.GroupOrWorldReadable {
		t.Errorf("0660 file should be flagged for group access: %+v", res)
	}
}

func TestRepoLocalRegistryPath(t *testing.T) {
	got := RepoLocalRegistryPath("/repo")
	want := filepath.Join("/repo", ".vh-agent-harness", "redlines.local.yml")
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}
