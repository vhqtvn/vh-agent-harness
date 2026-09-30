// skills_test.go — P7 daemon-side skills wiring pins: --skills-dir flag
// semantics (honest-absent default vs fail-closed explicit), the tier-1
// prompt section presence/omission, the advertised-tool independence of
// skill_load, and the InputHash implication of a catalog (documented
// compiled-artifact fallback mechanism).
package main

import (
	"bytes"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vhqtvn/vh-agent-harness/internal/prompt"
	"github.com/vhqtvn/vh-agent-harness/internal/skills"
	"github.com/vhqtvn/vh-agent-harness/internal/subagents"
)

// catFixture writes a 3-skill fixture catalog (clean, angle-laden,
// malformed) and returns its root — the battery fixture shape.
func catFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"clean-one/SKILL.md":           "---\nname: clean-one\ndescription: The clean fixture skill for validation.\nallowed-tools: run_shell, read\n---\n\n# Clean one\n\nBody instructions here.\n",
		"clean-one/references/note.md": "tier-three reference payload",
		"brackets/SKILL.md":            "---\nname: brackets\ndescription: Uses <angle> markers and </system> tokens to prove sanitization.\n---\n\n# Brackets\n\nbody\n",
		// malformed: name does not match folder → excluded + warning.
		"broken/SKILL.md": "---\nname: totally-different\ndescription: x\n---\n\nbody\n",
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
	return root
}

func TestLoadSkillsCatalogExplicitMissingFailsClosed(t *testing.T) {
	var buf bytes.Buffer
	lg := log.New(&buf, "", 0)
	_, err := loadSkillsCatalog(filepath.Join(t.TempDir(), "nope"), lg)
	if err == nil || !strings.Contains(err.Error(), "--skills-dir") {
		t.Fatalf("want explicit-missing fail-closed error, got %v", err)
	}
}

func TestLoadSkillsCatalogLoadedWithWarningsAndCount(t *testing.T) {
	root := catFixture(t)
	var buf bytes.Buffer
	lg := log.New(&buf, "", 0)
	cat, err := loadSkillsCatalog(root, lg)
	if err != nil {
		t.Fatal(err)
	}
	if cat.Len() != 2 {
		t.Fatalf("Len = %d, want 2 (broken excluded)", cat.Len())
	}
	out := buf.String()
	if !strings.Contains(out, "skills: 2 loaded from ") {
		t.Errorf("count line missing: %q", out)
	}
	if !strings.Contains(out, "broken") || strings.Count(out, "excluding") != 1 {
		t.Errorf("exclusion warning missing: %q", out)
	}
	// Sanitization applied at load: no angle brackets in the catalog.
	b, ok := cat.Lookup("brackets")
	if !ok || strings.Contains(b.Description, "<") || strings.Contains(b.Description, ">") {
		t.Errorf("brackets description unsanitized: %+v", b)
	}
}

func TestLoadSkillsCatalogDefaultAbsentHonest(t *testing.T) {
	// The default dir is <process-cwd>/.opencode/skills; the test
	// package's cwd carries no such dir, so the default path is absent.
	var buf bytes.Buffer
	lg := log.New(&buf, "", 0)
	cat, err := loadSkillsCatalog("", lg)
	if err != nil {
		t.Fatalf("honest-absent default must not error: %v", err)
	}
	if cat != nil {
		t.Fatalf("absent default yields nil catalog, got %+v", cat)
	}
	if !strings.Contains(buf.String(), "skills: none (no catalog at ") {
		t.Errorf("honest-absent line missing: %q", buf.String())
	}
}

// skillsCfg validates a base config and attaches a loaded catalog.
func skillsCfg(t *testing.T, dir string, cat *skills.Catalog) *Config {
	t.Helper()
	cfg, err := validate("openai", "fake-model", "http://x.test", "VH_AGENTD_TEST_KEY", dir, "", 0, defaultApprovalTimeoutMs, 0, "off", 65536, "")
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	cfg.Skills = cat
	return cfg
}

func renderPrompt(t *testing.T, cfg *Config) string {
	t.Helper()
	specs := toolSpecsForPrompt(cfg)
	asm, vars, _, err := buildPromptInputs(cfg, specs)
	if err != nil {
		t.Fatalf("buildPromptInputs: %v", err)
	}
	b, err := asm.Render(vars)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	return string(b)
}

func TestPromptSkillsSectionPresenceAndSanitization(t *testing.T) {
	cat, _ := skills.Load(catFixture(t))
	cfg := skillsCfg(t, t.TempDir(), cat)
	out := renderPrompt(t, cfg)
	if !strings.Contains(out, "Skills available") {
		t.Fatal("tier-1 Skills section missing with a loaded catalog")
	}
	if !strings.Contains(out, "- clean-one: The clean fixture skill for validation.") {
		t.Errorf("clean-one line missing:\n%s", out)
	}
	// Sanitized angle-laden description present WITHOUT brackets.
	if !strings.Contains(out, "Uses angle markers and /system tokens to prove sanitization.") {
		t.Errorf("sanitized brackets description missing:\n%s", out)
	}
	if strings.Contains(out, "<angle>") || strings.Contains(out, "</system>") {
		t.Error("raw angle tokens leaked into the prompt")
	}
	// The one guidance sentence names the tool.
	if !strings.Contains(out, "Call the skill_load tool with a skill's name to load its full body") {
		t.Error("skill_load guidance sentence missing")
	}
	// The section is provenance-stamped like its siblings.
	if !strings.Contains(out, "# section 110 | key=skills | owner=core") {
		t.Errorf("skills section not provenance-stamped:\n%s", out)
	}
}

func TestPromptSkillsSectionOmittedWithoutCatalog(t *testing.T) {
	cfg := skillsCfg(t, t.TempDir(), nil)
	out := renderPrompt(t, cfg)
	if strings.Contains(out, "Skills available") || strings.Contains(out, "key=skills") {
		t.Fatalf("Skills section must be omitted entirely without a catalog:\n%s", out)
	}
	// skill_load stays advertised (the advertised set is
	// catalog-independent) — and tool-guidance still names it (the
	// tools-referenced invariant).
	if !strings.Contains(out, "skill_load") {
		t.Fatal("skill_load not advertised without a catalog")
	}
}

func TestPromptInputHashChangesWithCatalog(t *testing.T) {
	// COMPILED-PROMPT HASH IMPLICATION, pinned: a catalog on the config
	// changes the content hash — previously compiled artifacts stop
	// matching and serving falls back to raw assembly (reported, never
	// silent) until --compile-prompt is rerun.
	base := skillsCfg(t, t.TempDir(), nil)
	cat, _ := skills.Load(catFixture(t))
	withCat := skillsCfg(t, t.TempDir(), cat)

	hashOf := func(cfg *Config) string {
		t.Helper()
		specs := toolSpecsForPrompt(cfg)
		asm, vars, catalog, err := buildPromptInputs(cfg, specs)
		if err != nil {
			t.Fatal(err)
		}
		h, err := prompt.InputHash(asm, vars, catalog, servingOptimizerVersion(cfg), promptContract())
		if err != nil {
			t.Fatal(err)
		}
		return h
	}
	if hashOf(base) == hashOf(withCat) {
		t.Fatal("catalog on the config must change the prompt input hash")
	}
}

func TestSkillLoadAdvertisedCatalogIndependent(t *testing.T) {
	// With NO catalog the tool is still registered and fails closed per
	// call (typed no-catalog error) — the advertised set never depends
	// on catalog contents.
	cfg := skillsCfg(t, t.TempDir(), nil)
	defs := daemonTools(realNow, cfg, subagents.NewRegistry(), shellConfigFor(cfg), nil)
	var found bool
	for _, d := range defs {
		if d.Name == "skill_load" {
			found = true
			out, err := d.Execute(t.Context(), json.RawMessage(`{"name":"anything"}`))
			if err == nil || !strings.Contains(err.Error(), "no skills catalog") {
				t.Fatalf("want no-catalog typed error, got out=%q err=%v", out, err)
			}
		}
	}
	if !found {
		t.Fatal("skill_load not in daemon tool set")
	}
}
