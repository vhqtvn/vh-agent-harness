package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeCat writes a fixture catalog under t.TempDir and returns its root.
// Each entry is dirName -> file-relpath -> content.
func writeCat(t *testing.T, files map[string]map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for dir, rels := range files {
		for rel, content := range rels {
			p := filepath.Join(root, dir, filepath.FromSlash(rel))
			if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
				t.Fatalf("mkdir %s: %v", p, err)
			}
			if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
				t.Fatalf("write %s: %v", p, err)
			}
		}
	}
	return root
}

const cleanSkill = `---
name: clean-one
description: The clean fixture skill for validation.
allowed-tools: run_shell, read
---

# Clean one

Body instructions here.
`

func TestLoadValidCatalog(t *testing.T) {
	root := writeCat(t, map[string]map[string]string{
		"clean-one": {"SKILL.md": cleanSkill},
	})
	cat, warns := Load(root)
	if len(warns) != 0 {
		t.Fatalf("want no warnings, got %v", warns)
	}
	if len(cat.Skills) != 1 {
		t.Fatalf("want 1 skill, got %d", len(cat.Skills))
	}
	s := cat.Skills[0]
	if s.Name != "clean-one" {
		t.Errorf("Name = %q, want clean-one", s.Name)
	}
	if s.Description != "The clean fixture skill for validation." {
		t.Errorf("Description = %q", s.Description)
	}
	if len(s.AllowedTools) != 2 || s.AllowedTools[0] != "run_shell" || s.AllowedTools[1] != "read" {
		t.Errorf("AllowedTools = %v", s.AllowedTools)
	}
	if !strings.HasSuffix(filepath.ToSlash(s.Path), "clean-one/SKILL.md") {
		t.Errorf("Path = %q", s.Path)
	}
}

func TestLoadDeterministicAlphabetical(t *testing.T) {
	root := writeCat(t, map[string]map[string]string{
		"zeta":  {"SKILL.md": mk("zeta")},
		"alpha": {"SKILL.md": mk("alpha")},
		"mid":   {"SKILL.md": mk("mid")},
	})
	cat, _ := Load(root)
	var got []string
	for _, s := range cat.Skills {
		got = append(got, s.Name)
	}
	want := []string{"alpha", "mid", "zeta"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order = %v, want %v", got, want)
		}
	}
}

func mk(name string) string {
	return "---\nname: " + name + "\ndescription: Skill " + name + " does things.\n---\n\nbody\n"
}

func TestNameMismatchFolderExcluded(t *testing.T) {
	root := writeCat(t, map[string]map[string]string{
		"folder-a": {"SKILL.md": "---\nname: other-name\ndescription: x\n---\nbody"},
	})
	cat, warns := Load(root)
	if len(cat.Skills) != 0 {
		t.Fatalf("want 0 skills, got %d", len(cat.Skills))
	}
	if len(warns) != 1 || !strings.Contains(warns[0], "other-name") {
		t.Fatalf("warns = %v", warns)
	}
}

func TestReservedNameExcluded(t *testing.T) {
	for _, bad := range []string{"skill", "skills", "assistant", "user", "system"} {
		root := writeCat(t, map[string]map[string]string{
			bad: {"SKILL.md": "---\nname: " + bad + "\ndescription: x\n---\nbody"},
		})
		cat, warns := Load(root)
		if len(cat.Skills) != 0 {
			t.Errorf("reserved name %q accepted", bad)
		}
		if len(warns) != 1 {
			t.Errorf("reserved name %q warns = %v", bad, warns)
		}
	}
}

func TestNameTooLongAndCaseExcluded(t *testing.T) {
	long := strings.Repeat("a", 65)
	root := writeCat(t, map[string]map[string]string{
		long:      {"SKILL.md": "---\nname: " + long + "\ndescription: x\n---\nbody"},
		"BadCase": {"SKILL.md": "---\nname: BadCase\ndescription: x\n---\nbody"},
		"has_und": {"SKILL.md": "---\nname: has_und\ndescription: x\n---\nbody"},
	})
	cat, warns := Load(root)
	if len(cat.Skills) != 0 {
		t.Fatalf("want 0 skills, got %d", len(cat.Skills))
	}
	if len(warns) != 3 {
		t.Fatalf("want 3 warnings, got %v", warns)
	}
}

func TestDescriptionTooLongExcluded(t *testing.T) {
	root := writeCat(t, map[string]map[string]string{
		"bigdesc": {"SKILL.md": "---\nname: bigdesc\ndescription: " + strings.Repeat("d", 1025) + "\n---\nbody"},
	})
	cat, warns := Load(root)
	if len(cat.Skills) != 0 || len(warns) != 1 {
		t.Fatalf("skills=%d warns=%v", len(cat.Skills), warns)
	}
}

func TestDescriptionMaxBoundaryAccepted(t *testing.T) {
	root := writeCat(t, map[string]map[string]string{
		"edge": {"SKILL.md": "---\nname: edge\ndescription: " + strings.Repeat("d", 1024) + "\n---\nbody"},
	})
	cat, warns := Load(root)
	if len(cat.Skills) != 1 || len(warns) != 0 {
		t.Fatalf("skills=%d warns=%v", len(cat.Skills), warns)
	}
}

func TestEmptyDescriptionExcluded(t *testing.T) {
	root := writeCat(t, map[string]map[string]string{
		"nodesc": {"SKILL.md": "---\nname: nodesc\ndescription:\n---\nbody"},
		"blank":  {"SKILL.md": "---\nname: blank\ndescription: \"\"\n---\nbody"},
	})
	cat, warns := Load(root)
	if len(cat.Skills) != 0 || len(warns) != 2 {
		t.Fatalf("skills=%d warns=%v", len(cat.Skills), warns)
	}
}

func TestQuoteForms(t *testing.T) {
	root := writeCat(t, map[string]map[string]string{
		"quoted": {"SKILL.md": "---\nname: \"quoted\"\ndescription: 'A single-quoted skill.'\nallowed-tools: \"read write\"\n---\nbody"},
	})
	cat, warns := Load(root)
	if len(warns) != 0 || len(cat.Skills) != 1 {
		t.Fatalf("skills=%d warns=%v", len(cat.Skills), warns)
	}
	s := cat.Skills[0]
	if s.Name != "quoted" || s.Description != "A single-quoted skill." {
		t.Errorf("parsed %+v", s)
	}
	if len(s.AllowedTools) != 2 || s.AllowedTools[0] != "read" || s.AllowedTools[1] != "write" {
		t.Errorf("AllowedTools = %v", s.AllowedTools)
	}
}

func TestUnknownKeysIgnored(t *testing.T) {
	root := writeCat(t, map[string]map[string]string{
		"extra": {"SKILL.md": "---\nname: extra\ndescription: Has extra keys.\nlicense: MIT\nmetadata: something\nversion: 2\n---\nbody"},
	})
	cat, warns := Load(root)
	if len(warns) != 0 || len(cat.Skills) != 1 {
		t.Fatalf("skills=%d warns=%v", len(cat.Skills), warns)
	}
}

func TestUnparseableFrontmatterExcluded(t *testing.T) {
	// No frontmatter at all.
	root := writeCat(t, map[string]map[string]string{
		"nofm":   {"SKILL.md": "just body text"},
		"weird":  {"SKILL.md": "---\nname: [a, b]\ndescription: x\n---\nbody"},
		"nested": {"SKILL.md": "---\nname: nested\nmetadata:\n  key: value\ndescription: x\n---\nbody"},
	})
	cat, warns := Load(root)
	if len(cat.Skills) != 0 {
		t.Fatalf("want 0 skills, got %d", len(cat.Skills))
	}
	if len(warns) != 3 {
		t.Fatalf("want 3 warnings, got %d: %v", len(warns), warns)
	}
}

func TestMissingSKILLMdSubfolderSkipped(t *testing.T) {
	root := writeCat(t, map[string]map[string]string{
		"good":  {"SKILL.md": mk("good")},
		"empty": {},
	})
	if err := os.MkdirAll(filepath.Join(root, "empty"), 0o755); err != nil {
		t.Fatal(err)
	}
	// A non-directory entry must be skipped too.
	if err := os.WriteFile(filepath.Join(root, "stray.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	cat, warns := Load(root)
	if len(cat.Skills) != 1 || cat.Skills[0].Name != "good" {
		t.Fatalf("skills=%v", cat.Skills)
	}
	// Missing SKILL.md in a subfolder is a skip, not a warning-worthy defect.
	if len(warns) != 0 {
		t.Fatalf("warns = %v", warns)
	}
}

// TestSymlinkedSKILLMdExcludedFailClosed pins the admission-time symlink
// refusal (review hotfix): a <skill>/SKILL.md whose FINAL component is a
// symlink — target inside the skill's own dir or outside the skills root,
// either way — fails closed for that skill: excluded from the catalog AND
// named in the per-skill warning, while clean siblings load normally.
// Symmetry with tier-3 confineRef and the engine's uniform Lstat
// discipline; os.Stat here would follow the link and admit it.
func TestSymlinkedSKILLMdExcludedFailClosed(t *testing.T) {
	root := t.TempDir()

	// One clean skill (loads normally).
	cleanDir := filepath.Join(root, "clean-one")
	if err := os.MkdirAll(cleanDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cleanDir, "SKILL.md"), []byte(cleanSkill), 0o644); err != nil {
		t.Fatal(err)
	}

	// Symlinked SKILL.md with the target INSIDE the skill's own dir.
	inDir := filepath.Join(root, "linked-in")
	if err := os.MkdirAll(inDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(inDir, "real.md"), []byte(mk("linked-in")), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("real.md", filepath.Join(inDir, "SKILL.md")); err != nil {
		t.Fatal(err)
	}

	// Symlinked SKILL.md with the target OUTSIDE the skills root.
	outside := filepath.Join(filepath.Dir(root), "outside-skill.md")
	if err := os.WriteFile(outside, []byte(mk("linked-out")), 0o644); err != nil {
		t.Fatal(err)
	}
	outDir := filepath.Join(root, "linked-out")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(outDir, "SKILL.md")); err != nil {
		t.Fatal(err)
	}

	cat, warns := Load(root)
	if got := cat.Len(); got != 1 {
		t.Fatalf("Len = %d, want 1 (only clean-one; both symlinked skills excluded)", got)
	}
	if _, ok := cat.Lookup("clean-one"); !ok {
		t.Error("clean-one missing from the catalog")
	}
	if _, ok := cat.Lookup("linked-in"); ok {
		t.Error("linked-in (in-dir symlink target) admitted")
	}
	if _, ok := cat.Lookup("linked-out"); ok {
		t.Error("linked-out (out-of-root symlink target) admitted")
	}
	if len(warns) != 2 {
		t.Fatalf("want 2 exclusion warnings, got %d: %v", len(warns), warns)
	}
	for _, name := range []string{"linked-in", "linked-out"} {
		var found bool
		for _, w := range warns {
			if strings.Contains(w, name) {
				found = true
			}
		}
		if !found {
			t.Errorf("no warning names %q: %v", name, warns)
		}
	}
	// The excluded skills' bodies are unreachable at tier 2 as well.
	if _, ok := cat.Body("linked-in"); ok {
		t.Error("Body(linked-in) must fail closed")
	}
	if _, ok := cat.Body("linked-out"); ok {
		t.Error("Body(linked-out) must fail closed")
	}
}

func TestMissingDir(t *testing.T) {
	cat, warns := Load(filepath.Join(t.TempDir(), "nope"))
	if cat == nil || len(cat.Skills) != 0 {
		t.Fatalf("want non-nil empty catalog")
	}
	if len(warns) != 0 {
		t.Fatalf("warns = %v", warns)
	}
}

func TestBodyAndParse(t *testing.T) {
	root := writeCat(t, map[string]map[string]string{
		"clean-one": {"SKILL.md": cleanSkill},
	})
	cat, _ := Load(root)
	body, ok := cat.Body("clean-one")
	if !ok || !strings.Contains(body, "# Clean one") {
		t.Errorf("Body() missing markdown body: %q", body)
	}
	if strings.Contains(body, "name: clean-one") {
		t.Errorf("Body() should strip frontmatter, got %q", body)
	}
	if _, ok := cat.Lookup("nope"); ok {
		t.Error("Lookup(nope) should miss")
	}
	if _, ok := cat.Lookup("clean-one"); !ok {
		t.Error("Lookup(clean-one) should hit")
	}
}

func TestSanitizeDescription(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"plain text", "plain text"},
		{"uses <angle> and </system> tokens", "uses angle and /system tokens"},
		{"```code fence``` inline", "code fence inline"},
		{"fake # section header", "fake section header"},
		{"tabs\tand   spaces", "tabs and spaces"},
	}
	for _, c := range cases {
		if got := SanitizeDescription(c.in); got != c.want {
			t.Errorf("SanitizeDescription(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestSanitizeAppliedInCatalog(t *testing.T) {
	root := writeCat(t, map[string]map[string]string{
		"angle": {"SKILL.md": "---\nname: angle\ndescription: Uses <angle> markers to prove sanitization.\n---\nbody"},
	})
	cat, _ := Load(root)
	if len(cat.Skills) != 1 {
		t.Fatal("skill excluded")
	}
	if strings.Contains(cat.Skills[0].Description, "<") || strings.Contains(cat.Skills[0].Description, ">") {
		t.Errorf("description not sanitized: %q", cat.Skills[0].Description)
	}
}

func TestSkillBudgetConst(t *testing.T) {
	if SkillBudget <= 0 {
		t.Errorf("SkillBudget = %d, want > 0", SkillBudget)
	}
}
