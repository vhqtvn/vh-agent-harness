package substrate

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"testing/fstest"

	corpus "github.com/vhqtvn/vh-agent-harness" // root package: embed vars corpus.CoreFS etc.
)

// bareHarnessCommandRe matches the binary/command name written as the bare,
// generic `harness <subcommand>` instead of the canonical `vh-agent-harness`.
// The leading (^|[^\w-]) ensures the `harness` of `vh-agent-harness` (preceded
// by `-`) is NOT matched, and the verb list is restricted to UNAMBIGUOUS
// subcommands that never occur as the English noun "harness <word>" (so e.g.
// "the harness version" / "a harness install" / "harness state" stay legal).
var bareHarnessCommandRe = regexp.MustCompile(
	`(^|[^\w-])harness[ \t]+(exec|shell|doctor|guide|diff|uninstall|preflight|proposals|self-update|ssh-trust|smoke|upgrade|git )`)

// jsBareHarnessPrefixRe catches the shell-guard gate recognizing the wrapper by
// the bare `"harness "` / `'harness '` prefix (startsWith/includes) — the exact
// defect that surfaced the bare generic command form to agents because the gate's notion
// of the command name diverged from the installed binary (vh-agent-harness).
var jsBareHarnessPrefixRe = regexp.MustCompile(`["'` + "`" + `]harness `)

// TestCorpus_NoBareHarnessCommandReferences is the durable regression guard for
// the command-name contract: the binary is `vh-agent-harness`, never the generic
// `harness`. Agent-facing corpus files (commands, skills, the shell-guard gate,
// deny-rules) that say bare `harness <verb>` teach agents to invoke a command
// that does not exist on PATH — exactly the failure a human reported. This walks
// the FULL embedded corpus and fails on any bare-`harness` command form.
func TestCorpus_NoBareHarnessCommandReferences(t *testing.T) {
	coreSub, err := fs.Sub(corpus.CoreFS, corpus.CoreDir)
	if err != nil {
		t.Fatalf("fs.Sub(core): %v", err)
	}
	var violations []string
	_ = fs.WalkDir(coreSub, ".", func(p string, d fs.DirEntry, werr error) error {
		if werr != nil || d.IsDir() {
			return nil
		}
		body, rerr := fs.ReadFile(coreSub, p)
		if rerr != nil {
			return nil
		}
		for i, line := range strings.Split(string(body), "\n") {
			if bareHarnessCommandRe.MatchString(line) || jsBareHarnessPrefixRe.MatchString(line) {
				violations = append(violations, fmt.Sprintf("%s:%d: %s", p, i+1, strings.TrimSpace(line)))
			}
		}
		return nil
	})
	if len(violations) > 0 {
		t.Fatalf("bare `harness <cmd>` in corpus (must be `vh-agent-harness`): %d:\n%s",
			len(violations), strings.Join(violations, "\n"))
	}
}

// TestEmbedFSRenderer_SubstitutesAndStripsSuffix proves the embed-backed
// renderer performs the same {{ .var }} substitution and .tmpl suffix stripping
// as GoTemplateRenderer, but reading from an fs.FS.
func TestEmbedFSRenderer_SubstitutesAndStripsSuffix(t *testing.T) {
	src := fstest.MapFS{
		"README.md.tmpl": &fstest.MapFile{
			Data: []byte("# {{ .project_name }}\nprofile={{ .profile }}\n"),
		},
	}
	staging := t.TempDir()
	r := EmbedFSRenderer{Source: src}
	if err := r.Render(staging, RenderSpec{
		Answers: map[string]string{"project_name": "Toy", "profile": "supervised"},
	}); err != nil {
		t.Fatalf("render: %v", err)
	}
	if _, err := os.Stat(filepath.Join(staging, "README.md.tmpl")); err == nil {
		t.Fatalf(".tmpl suffix must be stripped from staged filename")
	}
	got := readStaged(t, staging, "README.md")
	if !strings.Contains(got, "# Toy") || !strings.Contains(got, "profile=supervised") {
		t.Fatalf("substitution failed; got:\n%s", got)
	}
}

// TestEmbedFSRenderer_PreservesNonTemplateFilesVerbatim proves non-.tmpl files
// are never parsed as Go templates, and a literal "{{" that is NOT a canonical
// harness sentinel survives byte-for-byte from the fs.FS. (Plain files DO go
// through the tight harness-token pass — see SubstituteHarnessTokens — but
// "{{ looks }}" is outside its allowlist, so it stays literal, which is the
// guard this test provides that the allowlist never over-reaches.)
func TestEmbedFSRenderer_PreservesNonTemplateFilesVerbatim(t *testing.T) {
	const literal = "this {{ looks }} like a template but must be copied verbatim\n"
	src := fstest.MapFS{
		"AGENTS.core.md": &fstest.MapFile{Data: []byte(literal)},
		"sub/nested.txt": &fstest.MapFile{Data: []byte("nested literal\n")},
	}
	staging := t.TempDir()
	r := EmbedFSRenderer{Source: src}
	if err := r.Render(staging, RenderSpec{Answers: map[string]string{}}); err != nil {
		t.Fatalf("render: %v", err)
	}
	if got := readStaged(t, staging, "AGENTS.core.md"); got != literal {
		t.Fatalf("non-.tmpl byte mismatch; got=%q want=%q", got, literal)
	}
	if got := readStaged(t, staging, "sub/nested.txt"); got != "nested literal\n" {
		t.Fatalf("nested mismatch; got=%q", got)
	}
}

// TestEmbedFSRenderer_ConditionalEvaluatesBool proves the bool-coercion
// semantics are identical to GoTemplateRenderer (a "false" string is falsy).
func TestEmbedFSRenderer_ConditionalEvaluatesBool(t *testing.T) {
	src := fstest.MapFS{
		"cmd.md.tmpl": &fstest.MapFile{
			Data: []byte("{{ if .features.backlog }}YES{{ else }}NO{{ end }}\n"),
		},
	}
	r := EmbedFSRenderer{Source: src}
	t.Run("true", func(t *testing.T) {
		staging := t.TempDir()
		if err := r.Render(staging, RenderSpec{
			Answers: map[string]string{"features.backlog": "true"},
		}); err != nil {
			t.Fatalf("render: %v", err)
		}
		if got := readStaged(t, staging, "cmd.md"); !strings.Contains(got, "YES") {
			t.Fatalf("backlog=true must include block; got:\n%s", got)
		}
	})
	t.Run("false", func(t *testing.T) {
		staging := t.TempDir()
		if err := r.Render(staging, RenderSpec{
			Answers: map[string]string{"features.backlog": "false"},
		}); err != nil {
			t.Fatalf("render: %v", err)
		}
		if got := readStaged(t, staging, "cmd.md"); !strings.Contains(got, "NO") {
			t.Fatalf("backlog=false must EXCLUDE block; got:\n%s", got)
		}
	})
}

// TestEmbedFSRenderer_Idempotent proves re-rendering identical inputs produces
// an identical staged tree.
func TestEmbedFSRenderer_Idempotent(t *testing.T) {
	src := fstest.MapFS{
		"a.md.tmpl": &fstest.MapFile{Data: []byte("name={{ .project_name }}\n")},
		"b.txt":     &fstest.MapFile{Data: []byte("literal\n")},
	}
	spec := RenderSpec{Answers: map[string]string{"project_name": "Z"}}
	r := EmbedFSRenderer{Source: src}
	s1, s2 := t.TempDir(), t.TempDir()
	if err := r.Render(s1, spec); err != nil {
		t.Fatalf("render s1: %v", err)
	}
	if err := r.Render(s2, spec); err != nil {
		t.Fatalf("render s2: %v", err)
	}
	for _, rel := range []string{"a.md", "b.txt"} {
		a, _ := os.ReadFile(filepath.Join(s1, rel))
		b, _ := os.ReadFile(filepath.Join(s2, rel))
		if string(a) != string(b) {
			t.Fatalf("non-idempotent for %s: %q != %q", rel, a, b)
		}
	}
}

// TestEmbedFSRenderer_NameIsEmbedFS confirms the lineage rendered_by identity.
func TestEmbedFSRenderer_NameIsEmbedFS(t *testing.T) {
	r := EmbedFSRenderer{}
	if got := r.Name(); got != "embed-fs" {
		t.Fatalf("Name: want embed-fs, got %q", got)
	}
}

// TestEmbedFSRenderer_NilSourceErrors proves a nil fs.FS is a hard error
// (fail-closed) rather than a silent no-op.
func TestEmbedFSRenderer_NilSourceErrors(t *testing.T) {
	r := EmbedFSRenderer{}
	if err := r.Render(t.TempDir(), RenderSpec{}); err == nil {
		t.Fatalf("nil Source must error")
	}
}

// TestEmbedFSRenderer_RendersRealEmbeddedCoreCorpus proves the embed directive
// in corpus.go actually embedded templates/core and that the renderer can walk
// every file in it into staging without error. This is the smoke check that the
// CLI's install source is wired and loadable.
func TestEmbedFSRenderer_RendersRealEmbeddedCoreCorpus(t *testing.T) {
	coreSub, err := fs.Sub(corpus.CoreFS, corpus.CoreDir)
	if err != nil {
		t.Fatalf("fs.Sub(core): %v", err)
	}
	staging := t.TempDir()
	r := EmbedFSRenderer{Source: coreSub}
	if err := r.Render(staging, RenderSpec{
		Answers: map[string]string{"project_name": "Toy Project"},
	}); err != nil {
		t.Fatalf("render embedded core corpus: %v", err)
	}
	// Spot-check a few canonical curated paths landed in staging.
	for _, rel := range []string{
		filepath.FromSlash(".vh-agent-harness/AGENTS.core.md"),
		filepath.FromSlash(".vh-agent-harness/vh-harness-profile.yml"),
		filepath.FromSlash(".opencode/repo-configs/forbidden-patterns.project.js"),
		filepath.FromSlash(".opencode/agents/build.md"),
		filepath.FromSlash(".opencode/commands/coordination.md"),
		filepath.FromSlash(".opencode/skills/gated-commit/SKILL.md"),
		filepath.FromSlash(".opencode/plugins/shell-guard.js"),
		filepath.FromSlash("docs/coordination/README.md"),
	} {
		if _, err := os.Stat(filepath.Join(staging, rel)); err != nil {
			t.Fatalf("expected staged core file missing: %s: %v", rel, err)
		}
	}
	// Count staged files and confirm it is non-trivial (the full curated corpus).
	count := 0
	_ = filepath.Walk(staging, func(_ string, info os.FileInfo, _ error) error {
		if info != nil && !info.IsDir() {
			count++
		}
		return nil
	})
	if count < 100 {
		t.Fatalf("staged core corpus suspiciously small: %d files", count)
	}
}

// TestEmbedFSRenderer_RealCoreCorpusResolvesHarnessTokens is the durable
// FINDING #4 regression guard: render the FULL embedded templates/core corpus
// with sample answers and assert NO canonical harness sentinel
// ({{PROJECT_NAME}} / {{PROJECT_SLUG}} / {{COORDINATOR_DIR}}) ships literal in
// ANY staged file. This catches the defect across the whole shipped corpus, not
// just the synthetic case — if a future template edit reintroduces a literal
// sentinel that the renderer fails to resolve, this test fails.
func TestEmbedFSRenderer_RealCoreCorpusResolvesHarnessTokens(t *testing.T) {
	coreSub, err := fs.Sub(corpus.CoreFS, corpus.CoreDir)
	if err != nil {
		t.Fatalf("fs.Sub(core): %v", err)
	}
	staging := t.TempDir()
	r := EmbedFSRenderer{Source: coreSub}
	if err := r.Render(staging, RenderSpec{
		Answers: map[string]string{
			"project_name":    "Toy Project",
			"project_slug":    "toy-project",
			"coordinator_dir": "toy-coord",
		},
	}); err != nil {
		t.Fatalf("render embedded core corpus: %v", err)
	}
	sentinels := []string{
		harnessTokenProjectName,
		harnessTokenProjectSlug,
		harnessTokenCoordinatorDir,
		harnessTokenMissionSummary,
		harnessTokenArchSummary,
		harnessTokenDBUser,
		harnessTokenDBName,
	}
	var violations []string
	_ = filepath.Walk(staging, func(p string, info os.FileInfo, werr error) error {
		if werr != nil || info == nil || info.IsDir() {
			return nil
		}
		body, rerr := os.ReadFile(p)
		if rerr != nil {
			return nil
		}
		for _, s := range sentinels {
			if strings.Contains(string(body), s) {
				rel, _ := filepath.Rel(staging, p)
				violations = append(violations, fmt.Sprintf("%s contains literal %s", rel, s))
			}
		}
		return nil
	})
	if len(violations) > 0 {
		t.Fatalf("FINDING #4 regression: %d staged core file(s) still carry a literal harness sentinel:\n%s",
			len(violations), strings.Join(violations, "\n"))
	}
}
