package substrate

import (
	"testing"

	"github.com/vhqtvn/vh-agent-harness/internal/ownership"
)

func TestMatchGlob(t *testing.T) {
	cases := []struct {
		pattern, path string
		want          bool
	}{
		// Recursive /** matches dir + everything beneath.
		{".opencode/agents/**", ".opencode/agents", true},
		{".opencode/agents/**", ".opencode/agents/build.md", true},
		{".opencode/agents/**", ".opencode/agents/sub/a.md", true},
		{".opencode/agents/**", ".opencode/other.md", false},
		// Bare glob segments.
		{"*.md", "AGENTS.md", true},
		{"*.md", "docs/x.txt", false},
		// Single * does not cross segments.
		{"docs/*", "docs/a.md", true},
		{"docs/*", "docs/sub/a.md", false},
		// ** in the middle.
		{".opencode/**/SKILL.md", ".opencode/skills/x/SKILL.md", true},
		{".opencode/**/SKILL.md", ".opencode/SKILL.md", true}, // ** matches zero segments
		// Exact.
		{"vh-harness-profile.yml", "vh-harness-profile.yml", true},
		{"vh-harness-profile.yml", "other.yml", false},
	}
	for _, c := range cases {
		if got := matchGlob(c.pattern, c.path); got != c.want {
			t.Fatalf("matchGlob(%q, %q) = %v, want %v", c.pattern, c.path, got, c.want)
		}
	}
}

func TestClassifierExactThenGlobThenFailClosed(t *testing.T) {
	exact := ownership.ModuleDefaults{
		"vh-harness-profile.yml": {Class: ownership.ClassPlatformArmed, Provenance: "core"},
	}
	eff, err := ownership.Resolve(exact, nil)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	globs := []GlobRule{
		{Pattern: ".opencode/agents/**", Class: ownership.ClassPlatformManaged, Provenance: "core.agents"},
	}
	c := NewClassifier(eff, globs)

	// Exact hit wins.
	got, ok := c.Classify("vh-harness-profile.yml")
	if !ok || got.Class != ownership.ClassPlatformArmed || got.Origin != "exact" {
		t.Fatalf("exact classify: %+v ok=%v", got, ok)
	}
	// Glob hit.
	got, ok = c.Classify(".opencode/agents/build.md")
	if !ok || got.Class != ownership.ClassPlatformManaged || got.Origin != "glob" {
		t.Fatalf("glob classify: %+v ok=%v", got, ok)
	}
	// Fail-closed default.
	if _, ok := c.Classify("unknown.txt"); ok {
		t.Fatalf("unclassified path must be fail-closed (ok=false)")
	}
	// MustClassify surfaces the error.
	if _, err := c.MustClassify("unknown.txt"); err == nil {
		t.Fatalf("MustClassify must error on unclassified path")
	}
}

func TestClassifierAssumeFallback(t *testing.T) {
	c := NewClassifier(ownership.EffectiveMap{}, nil, WithUnclassifiedAssume(ownership.ClassPlatformManaged))
	got, ok := c.Classify("anything.md")
	if !ok || got.Class != ownership.ClassPlatformManaged || got.Origin != "assume" {
		t.Fatalf("assume classify: %+v ok=%v", got, ok)
	}
}
