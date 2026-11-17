package skillload

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vhqtvn/vh-agent-harness/internal/skills"
)

const cleanSkill = `---
name: clean-one
description: The clean fixture skill for validation.
allowed-tools: run_shell, read
---

# Clean one

Body instructions here.
`

const plainSkill = `---
name: plain-skill
description: A skill with no allowed-tools.
---

# Plain

Body.
`

func fixtureCat(t *testing.T) (*skills.Catalog, string) {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"clean-one/SKILL.md":              cleanSkill,
		"plain-skill/SKILL.md":            plainSkill,
		"clean-one/references/note.md":    "tier-three reference payload",
		"clean-one/references/deep/d.txt": "deep reference",
		"clean-one/scripts/inert.sh":      "#!/bin/sh\necho never auto-executes\n",
	}
	for rel, content := range files {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	cat, warns := skills.Load(root)
	if len(warns) != 0 || cat.Len() != 2 {
		t.Fatalf("fixture catalog broken: warns=%v len=%d", warns, cat.Len())
	}
	return cat, root
}

func run(t *testing.T, cat *skills.Catalog, prov Provenance, args string) (string, error) {
	t.Helper()
	d := Definition(cat, 64*1024, prov)
	return d.Execute(context.Background(), json.RawMessage(args))
}

func TestLoadWholeSkillBody(t *testing.T) {
	cat, _ := fixtureCat(t)
	out, err := run(t, cat, nil, `{"name":"clean-one"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "# Clean one") {
		t.Errorf("body missing: %q", out)
	}
	if strings.Contains(out, "name: clean-one") {
		t.Errorf("frontmatter should be stripped: %q", out)
	}
	if !strings.Contains(out, "allowed-tools ceiling: run_shell, read (intersected with the registry — never a grant)") {
		t.Errorf("ceiling footer missing: %q", out)
	}
}

func TestLoadNoneDeclaredFooter(t *testing.T) {
	cat, _ := fixtureCat(t)
	out, err := run(t, cat, nil, `{"name":"plain-skill"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "allowed-tools ceiling: none declared (intersected with the registry — never a grant)") {
		t.Errorf("none-declared footer missing: %q", out)
	}
}

func TestUnknownSkillFailClosed(t *testing.T) {
	cat, _ := fixtureCat(t)
	_, err := run(t, cat, nil, `{"name":"does-not-exist"}`)
	if err == nil || !strings.Contains(err.Error(), `unknown skill "does-not-exist"`) {
		t.Fatalf("want typed unknown-skill error, got %v", err)
	}
}

func TestNoCatalogFailClosed(t *testing.T) {
	_, err := run(t, nil, nil, `{"name":"clean-one"}`)
	if err == nil || !strings.Contains(err.Error(), "no skills catalog") {
		t.Fatalf("want no-catalog error, got %v", err)
	}
	empty := &skills.Catalog{}
	_, err = run(t, empty, nil, `{"name":"clean-one"}`)
	if err == nil || !strings.Contains(err.Error(), "unknown skill") {
		t.Fatalf("want unknown-skill error on empty catalog, got %v", err)
	}
}

func TestSymlinkedSkillNotLoadableTier2(t *testing.T) {
	// Review hotfix: a skill whose SKILL.md is a symlink at admission is
	// EXCLUDED from the catalog, so skill_load fails closed with the
	// typed unknown-skill error — the body behind the symlink is
	// unreachable through the guarded tool; the clean sibling loads.
	root := t.TempDir()
	files := map[string]string{
		"clean-one/SKILL.md": cleanSkill,
		"linked/real.md":     "---\nname: linked\ndescription: Symlinked skill body.\n---\n\nsymlinked body\n",
	}
	for rel, content := range files {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink("real.md", filepath.Join(root, "linked", "SKILL.md")); err != nil {
		t.Fatal(err)
	}
	cat, warns := skills.Load(root)
	if cat.Len() != 1 || len(warns) != 1 {
		t.Fatalf("fixture broken: len=%d warns=%v (symlinked skill must be excluded)", cat.Len(), warns)
	}
	_, err := run(t, cat, nil, `{"name":"linked"}`)
	if err == nil || !strings.Contains(err.Error(), `unknown skill "linked"`) {
		t.Fatalf("want typed unknown-skill error for the symlink-excluded skill, got %v", err)
	}
	if _, err := run(t, cat, nil, `{"name":"clean-one"}`); err != nil {
		t.Fatalf("clean-one must still load through the same catalog: %v", err)
	}
}

func TestRefLoadTier3(t *testing.T) {
	cat, _ := fixtureCat(t)
	out, err := run(t, cat, nil, `{"name":"clean-one","ref":"references/note.md"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "tier-three reference payload") {
		t.Errorf("ref content missing: %q", out)
	}
	if !strings.Contains(out, "allowed-tools ceiling: run_shell, read") {
		t.Errorf("ceiling footer missing on ref load: %q", out)
	}
	// Nested ref under a subdirectory.
	out, err = run(t, cat, nil, `{"name":"clean-one","ref":"references/deep/d.txt"}`)
	if err != nil || !strings.Contains(out, "deep reference") {
		t.Errorf("nested ref: out=%q err=%v", out, err)
	}
}

func TestRefTraversalRejected(t *testing.T) {
	cat, root := fixtureCat(t)
	// Escape target outside the skill dir.
	outside := filepath.Join(filepath.Dir(root), "secret.txt")
	if err := os.WriteFile(outside, []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, ref := range []string{"../../secret.txt", "../../../etc/passwd", "references/../../../../secret.txt"} {
		if _, err := run(t, cat, nil, fmt.Sprintf(`{"name":"clean-one","ref":%q}`, ref)); err == nil {
			t.Errorf("traversal ref %q accepted", ref)
		} else if !strings.Contains(err.Error(), "confinement") {
			t.Errorf("ref %q: want confinement error, got %v", ref, err)
		}
	}
}

func TestRefAbsoluteRejected(t *testing.T) {
	cat, _ := fixtureCat(t)
	if _, err := run(t, cat, nil, `{"name":"clean-one","ref":"/etc/passwd"}`); err == nil {
		t.Error("absolute ref accepted")
	}
}

func TestRefSymlinkEscapeRejected(t *testing.T) {
	cat, root := fixtureCat(t)
	outside := filepath.Join(filepath.Dir(root), "outside.md")
	if err := os.WriteFile(outside, []byte("outside"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "clean-one", "references", "escape.md")); err != nil {
		t.Fatal(err)
	}
	// Final-component symlink.
	if _, err := run(t, cat, nil, `{"name":"clean-one","ref":"references/escape.md"}`); err == nil {
		t.Error("symlink-final ref accepted")
	}
	// Intermediate symlink dir pointing outside.
	if err := os.Symlink(filepath.Dir(root), filepath.Join(root, "clean-one", "linked")); err != nil {
		t.Fatal(err)
	}
	if _, err := run(t, cat, nil, `{"name":"clean-one","ref":"linked/outside.md"}`); err == nil {
		t.Error("symlink-dir escape ref accepted")
	}
}

func TestRefCrossSkillRejected(t *testing.T) {
	cat, _ := fixtureCat(t)
	// Lexically inside the catalog root, OUTSIDE the named skill's dir.
	if _, err := run(t, cat, nil, `{"name":"clean-one","ref":"../plain-skill/SKILL.md"}`); err == nil {
		t.Error("cross-skill ref accepted")
	}
}

func TestRefOversizeFailClosed(t *testing.T) {
	cat, root := fixtureCat(t)
	big := filepath.Join(root, "clean-one", "references", "big.txt")
	if err := os.WriteFile(big, []byte(strings.Repeat("x", 65*1024)), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := run(t, cat, nil, `{"name":"clean-one","ref":"references/big.txt"}`)
	if err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("want oversize typed error, got %v", err)
	}
}

func TestRefMissingAndDirectory(t *testing.T) {
	cat, _ := fixtureCat(t)
	if _, err := run(t, cat, nil, `{"name":"clean-one","ref":"references/nope.md"}`); err == nil {
		t.Error("missing ref accepted")
	}
	if _, err := run(t, cat, nil, `{"name":"clean-one","ref":"references"}`); err == nil {
		t.Error("directory ref accepted")
	}
}

func TestProvenanceCallback(t *testing.T) {
	cat, _ := fixtureCat(t)
	type rec struct{ name, ref, sha string }
	var got []rec
	prov := func(ctx context.Context, name, ref, sha string) { got = append(got, rec{name, ref, sha}) }

	out, err := run(t, cat, prov, `{"name":"clean-one"}`)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte(contentPortion(out)))
	if len(got) != 1 || got[0].name != "clean-one" || got[0].ref != "" || got[0].sha != hex.EncodeToString(sum[:]) {
		t.Fatalf("provenance records = %v", got)
	}

	got = nil
	if _, err := run(t, cat, prov, `{"name":"clean-one","ref":"references/note.md"}`); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ref != "references/note.md" {
		t.Fatalf("ref provenance records = %v", got)
	}

	// Failures never call provenance.
	got = nil
	_, _ = run(t, cat, prov, `{"name":"nope"}`)
	if len(got) != 0 {
		t.Fatalf("provenance on failure = %v", got)
	}
}

// contentPortion strips the trailing ceiling footer to recover the exact
// content the model received (sha basis).
func contentPortion(out string) string {
	if i := strings.LastIndex(out, "\n\n---\n"); i >= 0 {
		return out[:i]
	}
	return out
}

func TestArgsValidation(t *testing.T) {
	cat, _ := fixtureCat(t)
	if _, err := run(t, cat, nil, `{}`); err == nil || !strings.Contains(err.Error(), "name is required") {
		t.Fatalf("want name-required error, got %v", err)
	}
	if _, err := run(t, cat, nil, `{"name":"clean-one","bogus":1}`); err == nil {
		t.Error("unknown arg field accepted")
	}
}
