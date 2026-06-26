package substrate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeCorpusFile writes a file under a template-root temp dir (used to build
// ad-hoc template trees for the renderer engine tests).
func writeCorpusFile(t *testing.T, root, rel, content string) {
	t.Helper()
	p := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", p, err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", p, err)
	}
}

func readStaged(t *testing.T, staging, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(staging, rel))
	if err != nil {
		t.Fatalf("read staged %s: %v", rel, err)
	}
	return string(b)
}

// TestGoTemplateRenderer_SubstitutesAndStripsSuffix proves the engine performs
// {{ .var }} token substitution and strips the .tmpl suffix from the staged
// filename. This is the core feature the corpus needs at Slice 2 and that the
// backlog conditional (Slice 3) builds on.
func TestGoTemplateRenderer_SubstitutesAndStripsSuffix(t *testing.T) {
	root := t.TempDir()
	staging := t.TempDir()
	writeCorpusFile(t, root, "README.md.tmpl",
		"# {{ .project_name }} (slug: {{ .project_slug }})\nprofile={{ .profile }}\n")

	r := GoTemplateRenderer{TemplateRoot: root}
	if err := r.Render(staging, RenderSpec{
		Answers: map[string]string{"project_name": "Toy Harness", "profile": "supervised"},
	}); err != nil {
		t.Fatalf("render: %v", err)
	}
	// Suffix stripped from the staged name.
	if _, err := os.Stat(filepath.Join(staging, "README.md.tmpl")); err == nil {
		t.Fatalf(".tmpl suffix must be stripped from staged filename")
	}
	got := readStaged(t, staging, "README.md")
	if !strings.Contains(got, "# Toy Harness") {
		t.Fatalf("project_name not substituted; got:\n%s", got)
	}
	// project_slug derived from project_name (lower + spaces->dashes).
	if !strings.Contains(got, "slug: toy-harness") {
		t.Fatalf("project_slug not derived; got:\n%s", got)
	}
	if !strings.Contains(got, "profile=supervised") {
		t.Fatalf("profile not substituted; got:\n%s", got)
	}
}

// TestGoTemplateRenderer_ConditionalEvaluatesBool proves {{ if .features.backlog
// }}...{{ end }} evaluates the BOOLEAN (not a non-empty string). This is the
// Slice-3 backlog-conditional feature the renderer must support from Slice 1,
// and it guards the bool-coercion logic in buildTemplateData (a literal "false"
// string would otherwise be truthy).
func TestGoTemplateRenderer_ConditionalEvaluatesBool(t *testing.T) {
	root := t.TempDir()
	writeCorpusFile(t, root, "cmd.md.tmpl",
		"commands:\n{{ if .features.backlog }}  - backlog-cleanup\n{{ end }}\n")

	t.Run("backlog_true_includes_block", func(t *testing.T) {
		staging := t.TempDir()
		r := GoTemplateRenderer{TemplateRoot: root}
		if err := r.Render(staging, RenderSpec{
			Answers: map[string]string{"features.backlog": "true"},
		}); err != nil {
			t.Fatalf("render: %v", err)
		}
		got := readStaged(t, staging, "cmd.md")
		if !strings.Contains(got, "- backlog-cleanup") {
			t.Fatalf("backlog=true must include the block; got:\n%s", got)
		}
	})
	t.Run("backlog_false_excludes_block", func(t *testing.T) {
		staging := t.TempDir()
		r := GoTemplateRenderer{TemplateRoot: root}
		if err := r.Render(staging, RenderSpec{
			Answers: map[string]string{"features.backlog": "false"},
		}); err != nil {
			t.Fatalf("render: %v", err)
		}
		got := readStaged(t, staging, "cmd.md")
		if strings.Contains(got, "backlog-cleanup") {
			t.Fatalf("backlog=false must EXCLUDE the block (bool coercion, not string truthiness); got:\n%s", got)
		}
	})
	t.Run("backlog_absent_excludes_block", func(t *testing.T) {
		staging := t.TempDir()
		r := GoTemplateRenderer{TemplateRoot: root}
		// No features.backlog answer at all: features map is seeded empty,
		// .features.backlog is nil -> falsy -> block excluded. missingkey=error
		// does NOT fire for map keys that exist (features exists; backlog is
		// the zero value of a missing inner key, which text/template treats as
		// <no value> / nil under map access without error).
		if err := r.Render(staging, RenderSpec{
			Answers: map[string]string{"project_name": "x"},
		}); err != nil {
			t.Fatalf("render: %v", err)
		}
		got := readStaged(t, staging, "cmd.md")
		if strings.Contains(got, "backlog-cleanup") {
			t.Fatalf("absent backlog flag must EXCLUDE the block; got:\n%s", got)
		}
	})
}

// TestGoTemplateRenderer_PreservesNonTemplateFilesVerbatim proves the "render vs
// preserve-as-is" rule: files WITHOUT the .tmpl suffix are never parsed as Go
// templates (no {{ .x }} execution), and a literal "{{" that is NOT one of the
// three canonical harness sentinels must survive untouched. (Plain files DO
// undergo a separate, tightly-allowlisted harness-token pass that resolves only
// {{PROJECT_NAME}}/{{PROJECT_SLUG}}/{{COORDINATOR_DIR}} — see
// substituteHarnessTokens (now exported SubstituteHarnessTokens). "{{ looks }}" is not in that allowlist, so it stays
// byte-identical, which is exactly the guard this test provides.)
func TestGoTemplateRenderer_PreservesNonTemplateFilesVerbatim(t *testing.T) {
	root := t.TempDir()
	staging := t.TempDir()
	const literal = "this {{ looks }} like a template but must be copied verbatim\n"
	writeCorpusFile(t, root, "AGENTS.core.md", literal)            // no .tmpl suffix
	writeCorpusFile(t, root, "sub/nested.txt", "nested literal\n") // nested dir, no .tmpl

	r := GoTemplateRenderer{TemplateRoot: root}
	if err := r.Render(staging, RenderSpec{Answers: map[string]string{}}); err != nil {
		t.Fatalf("render: %v", err)
	}
	if got := readStaged(t, staging, "AGENTS.core.md"); got != literal {
		t.Fatalf("non-.tmpl file must be byte-identical; got=%q want=%q", got, literal)
	}
	if got := readStaged(t, staging, "sub/nested.txt"); got != "nested literal\n" {
		t.Fatalf("nested non-.tmpl file mismatch; got=%q", got)
	}
}

// TestGoTemplateRenderer_Idempotent proves re-rendering identical inputs
// produces an identical staged tree (the Renderer contract).
func TestGoTemplateRenderer_Idempotent(t *testing.T) {
	root := t.TempDir()
	writeCorpusFile(t, root, "a.md.tmpl", "name={{ .project_name }}\n")
	writeCorpusFile(t, root, "b.txt", "literal\n")

	spec := RenderSpec{Answers: map[string]string{"project_name": "Z"}}
	r := GoTemplateRenderer{TemplateRoot: root}

	s1 := t.TempDir()
	if err := r.Render(s1, spec); err != nil {
		t.Fatalf("render s1: %v", err)
	}
	s2 := t.TempDir()
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

// TestGoTemplateRenderer_UndefinedFlagIsFalsyNotError proves the Jinja-like
// behavior the corpus conditionals rely on: a conditional referencing an unset
// flag ({{ if .features.X }}) evaluates to FALSY (block excluded), NOT an
// error. This matches Copier/Jinja treatment of undefined answers and is the
// semantics Slice-3 backlog conditionals expect. (We deliberately do NOT use
// missingkey=error here, since that would make a missing nested key error
// instead of being falsy.) A direct substitution of an undefined value yields
// Go's default "<no value>" — the corpus answer contract supplies every
// substituted variable, so this never surfaces for a well-formed corpus.
func TestGoTemplateRenderer_UndefinedFlagIsFalsyNotError(t *testing.T) {
	root := t.TempDir()
	staging := t.TempDir()
	writeCorpusFile(t, root, "x.md.tmpl", "{{ if .features.never_set }}YES{{ else }}NO{{ end }}\n")

	r := GoTemplateRenderer{TemplateRoot: root}
	if err := r.Render(staging, RenderSpec{Answers: map[string]string{"project_name": "y"}}); err != nil {
		t.Fatalf("absent flag must not error (Jinja-falsy semantics); got: %v", err)
	}
	got := readStaged(t, staging, "x.md")
	if !strings.Contains(got, "NO") || strings.Contains(got, "YES") {
		t.Fatalf("absent flag must be falsy (block excluded); got:\n%s", got)
	}
}

// TestGoTemplateRenderer_NameIsGoTemplate confirms the lineage rendered_by
// identity the seam records.
func TestGoTemplateRenderer_NameIsGoTemplate(t *testing.T) {
	r := GoTemplateRenderer{}
	if got := r.Name(); got != "go-template" {
		t.Fatalf("Name: want go-template, got %q", got)
	}
}

// TestSubstituteHarnessTokens pins the exact resolution semantics of the
// harness-token pass: the three canonical sentinels resolve from answers
// (with PROJECT_SLUG case-aware and COORDINATOR_DIR defaulting), while every
// other {{...}} form (soft fill-in-by-hand placeholders, literal "{{ looks }}")
// survives untouched. This is the unit core of FINDING #4's fix.
func TestSubstituteHarnessTokens(t *testing.T) {
	t.Run("project_name_verbatim", func(t *testing.T) {
		got := SubstituteHarnessTokens([]byte("hello {{PROJECT_NAME}}!"), map[string]string{"project_name": "Toy"})
		if string(got) != "hello Toy!" {
			t.Fatalf("got=%q", got)
		}
	})
	t.Run("coordinator_dir_default_when_unset", func(t *testing.T) {
		// No coordinator_dir answer -> default "coordinator".
		got := SubstituteHarnessTokens([]byte(".local/{{COORDINATOR_DIR}}/x"), map[string]string{})
		if string(got) != ".local/coordinator/x" {
			t.Fatalf("default coordinator_dir failed; got=%q", got)
		}
	})
	t.Run("coordinator_dir_custom", func(t *testing.T) {
		got := SubstituteHarnessTokens([]byte(".local/{{COORDINATOR_DIR}}/"), map[string]string{"coordinator_dir": "ops"})
		if string(got) != ".local/ops/" {
			t.Fatalf("got=%q", got)
		}
	})
	t.Run("project_slug_case_aware", func(t *testing.T) {
		in := []byte("secret: {{PROJECT_SLUG}}_JWT_SECRET\nimage: {{PROJECT_SLUG}}-dev-1\nplain {{PROJECT_SLUG}} end")
		got := SubstituteHarnessTokens(in, map[string]string{"project_slug": "toy"})
		want := "secret: TOY_JWT_SECRET\nimage: toy-dev-1\nplain toy end"
		if string(got) != want {
			t.Fatalf("case-aware slug failed;\ngot =%q\nwant=%q", got, want)
		}
	})
	t.Run("project_slug_eof_defaults_lower", func(t *testing.T) {
		// Sentinel at EOF (no trailing byte) -> lower (path/default context).
		got := SubstituteHarnessTokens([]byte("{{PROJECT_SLUG}}"), map[string]string{"project_slug": "Toy"})
		if string(got) != "toy" {
			t.Fatalf("EOF slug must default to lower; got=%q", got)
		}
	})
	t.Run("project_slug_derived_from_project_name_when_unset", func(t *testing.T) {
		got := SubstituteHarnessTokens([]byte("{{PROJECT_SLUG}}-x"), map[string]string{"project_name": "Toy Harness"})
		if string(got) != "toy-harness-x" {
			t.Fatalf("derived slug failed; got=%q", got)
		}
	})
	t.Run("non_allowlist_tokens_and_literal_braces_untouched", func(t *testing.T) {
		in := []byte("custom={{CUSTOM_TOKEN}} other={{SOMETHING_ELSE}} literal {{ looks }} like a template")
		got := SubstituteHarnessTokens(in, map[string]string{"project_name": "Toy"})
		if string(got) != string(in) {
			t.Fatalf("non-allowlist tokens must survive byte-identical;\ngot =%q\nwant=%q", got, in)
		}
	})
	t.Run("project_config_tokens_resolve", func(t *testing.T) {
		in := []byte("m={{MISSION_SUMMARY}} a={{ARCHITECTURE_SUMMARY}} u={{DB_USER}} n={{DB_NAME}}")
		got := SubstituteHarnessTokens(in, map[string]string{
			"mission_summary": "M", "architecture_summary": "A", "db_user": "U", "db_name": "N",
		})
		if want := "m=M a=A u=U n=N"; string(got) != want {
			t.Fatalf("project.config tokens must resolve;\ngot =%q\nwant=%q", got, want)
		}
	})
	t.Run("fast_path_noop_when_no_sentinel", func(t *testing.T) {
		in := []byte("nothing to see here, just literal {{ braces }} and text")
		got := SubstituteHarnessTokens(in, map[string]string{"project_name": "Toy"})
		// Must return the SAME slice (no allocation) when no canonical sentinel is present.
		if &got[0] != &in[0] {
			t.Fatalf("fast path must return body unchanged (same slice); got a copy")
		}
	})
}

// TestGoTemplateRenderer_ResolvesHarnessTokenSentinels proves the pass runs on
// PLAIN (non-.tmpl) shipped files end-to-end: a shipped command file carrying
// the three canonical sentinels renders token-free, while a soft placeholder
// and a literal "{{ looks }}" in the same file survive untouched. This is the
// durable guard against the FINDING #4 defect (a literal {{COORDINATOR_DIR}}
// shipping into a consumer's tree).
func TestGoTemplateRenderer_ResolvesHarnessTokenSentinels(t *testing.T) {
	root := t.TempDir()
	staging := t.TempDir()
	writeCorpusFile(t, root, "commands.md",
		"# {{PROJECT_NAME}}\n"+
			"secret: {{PROJECT_SLUG}}_JWT_SECRET\n"+
			"image: {{PROJECT_SLUG}}-dev-1\n"+
			"state: .local/{{COORDINATOR_DIR}}/\n"+
			"note={{CUSTOM_NOTE}} (fill in by hand)\n"+
			"literal {{ looks }} like a template\n")

	r := GoTemplateRenderer{TemplateRoot: root}
	if err := r.Render(staging, RenderSpec{
		Answers: map[string]string{
			"project_name":    "Toy Project",
			"project_slug":    "toy-project",
			"coordinator_dir": "toy-coord",
		},
	}); err != nil {
		t.Fatalf("render: %v", err)
	}
	got := readStaged(t, staging, "commands.md")
	for _, bad := range []string{
		harnessTokenProjectName, harnessTokenProjectSlug, harnessTokenCoordinatorDir,
	} {
		if strings.Contains(got, bad) {
			t.Fatalf("sentinel %q must NOT ship literal; got:\n%s", bad, got)
		}
	}
	for _, want := range []string{
		"# Toy Project",
		"secret: TOY-PROJECT_JWT_SECRET", // '_' after -> UPPER
		"image: toy-project-dev-1",       // '-' after -> lower
		"state: .local/toy-coord/",
		"note={{CUSTOM_NOTE}} (fill in by hand)", // non-harness token preserved
		"literal {{ looks }} like a template",    // literal braces preserved
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected %q in rendered output; got:\n%s", want, got)
		}
	}
}
