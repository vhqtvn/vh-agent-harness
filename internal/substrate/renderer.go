package substrate

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	texttemplate "text/template"
)

// RenderSpec describes one render of a template into a staging directory. The
// GoTemplateRenderer walks a local template root and renders it into staging.
type RenderSpec struct {
	// TemplateSource is the local template directory rendered into staging. For
	// GoTemplateRenderer / FixtureRenderer it is a filesystem path.
	TemplateSource string
	// Answers are the render answers (e.g. project_name, project_slug, profile,
	// and dotted feature flags such as features.backlog). GoTemplateRenderer
	// exposes them to the template engine as a nested data context.
	Answers map[string]string
	// Commit pins the template commit (optional; empty when not pinned).
	Commit string
	// Ref is the symbolic template ref (optional). For the Go-native renderer
	// the harness records its OWN bundled-template version here (the Copier
	// --vcs-ref git-tag model is retired: the version comes from the harness,
	// not a git tag on templates/core/).
	Ref string
}

// Renderer renders a template into a staging directory. The seam NEVER renders
// into the live project tree: the render engine's (uniform) output is confined
// to the staging directory, and the per-class apply reads from staging. This is
// the seam's one abstraction over the render mechanism, so a fast test renderer
// can substitute for the production one without touching apply/reconcile logic.
type Renderer interface {
	// Render renders the template into stagingDir (created if absent). It MUST
	// be idempotent for identical (spec, stagingDir): re-rendering produces the
	// same staged tree.
	Render(stagingDir string, spec RenderSpec) error
	// Name identifies the renderer for lineage/reporting (e.g. "go-template" or
	// "fixture-test-renderer"). The seam records this in lineage.yml as the
	// S1 rendered_by fact.
	Name() string
}

// TemplateSuffix is the filename suffix marking a file as a Go text/template.
// A file whose name ends in TemplateSuffix is parsed and executed as a
// template; the suffix is stripped from the staged output filename. Every other
// file is copied verbatim — it is never parsed as a template, so a document
// that happens to contain a literal "{{" is safe. This mirrors Copier's
// _templates_suffix: .jinja convention and is the renderer's "render vs
// preserve-as-is" rule.
const TemplateSuffix = ".tmpl"

// TemplateAltSuffix is an alternative template suffix. A file whose name ends in
// TemplateAltSuffix is a STATIC-SCAFFOLD template: its OUTPUT NAME is the
// suffix-stripped form (so Makefile.template stages as Makefile), but its BODY
// is copied VERBATIM — it is NOT parsed through text/template. The corpus's
// .template files use {{ALL_CAPS}} sentinel placeholders (e.g. {{PROJECT_SLUG}})
// that are resolved by hand or a downstream installer, never by the Go template
// engine (which would choke on a bare {{PROJECT_SLUG}} action). This keeps
// .template files consistent with every other static-scaffold corpus file: name
// finalized at render, body left untouched.
const TemplateAltSuffix = ".template"

// fileIsStaticTemplate reports whether name ends in the static-scaffold suffix
// (.template). Such files are suffix-stripped on stage but copied verbatim.
func fileIsStaticTemplate(name string) bool {
	return strings.HasSuffix(name, TemplateAltSuffix)
}

// stripTemplateSuffix returns name with one trailing template suffix removed
// (.template first, then .tmpl). If neither matches, name is returned as-is.
func stripTemplateSuffix(name string) string {
	if s := strings.TrimSuffix(name, TemplateAltSuffix); s != name {
		return s
	}
	return strings.TrimSuffix(name, TemplateSuffix)
}

// renderWriteMode returns the on-disk permission bits for a staged/live file
// based on its (suffix-stripped) name: shell scripts are executable (0o755),
// every other file is a normal file (0o644). The embed.FS embeds regular files
// read-only, so the mode is derived from the name rather than copied from the
// source; this preserves the exec bit the corpus authors intended for scripts
// (Makefile, .opencode/scripts/*.sh) across the render→stage→apply→install path.
func renderWriteMode(name string) os.FileMode {
	if strings.HasSuffix(name, ".sh") {
		return 0o755
	}
	return 0o644
}

// GoTemplateRenderer is the production Go-native renderer. It renders the
// bundled template corpus into a staging directory using the standard library's
// text/template engine — no Python, no Copier, no third-party templating
// dependency. A single self-contained Go binary renders the corpus.
//
// The corpus template files use Go text/template syntax: {{ .project_name }}
// substitution, {{ .profile }} enum answer, and {{ if .features.backlog }}
// ... {{ end }} conditionals (the backlog conditional lands in Slice 3 but the
// renderer supports it from Slice 1).
//
// See buildTemplateData for the answer -> template data mapping (flat keys,
// dotted-key nesting, and bool coercion so {{ if .features.backlog }} evaluates
// the boolean rather than a non-empty string).
type GoTemplateRenderer struct {
	// TemplateRoot is the local directory whose tree is rendered into staging.
	// If empty, spec.TemplateSource is used.
	TemplateRoot string
	// TemplateSuffix overrides the default TemplateSuffix when non-empty.
	TemplateSuffix string
}

// Name implements Renderer.
func (GoTemplateRenderer) Name() string { return "go-template" }

// Render walks the TemplateRoot, renders *.tmpl files through text/template into
// stagingDir (suffix stripped from the staged name), and copies every other
// file verbatim. Directories are mirrored. The walk is deterministic
// (filepath.WalkDir visits lexically) so identical inputs stage identical trees.
func (r GoTemplateRenderer) Render(stagingDir string, spec RenderSpec) error {
	root := r.TemplateRoot
	if root == "" {
		root = spec.TemplateSource
	}
	suffix := r.TemplateSuffix
	if suffix == "" {
		suffix = TemplateSuffix
	}
	info, err := os.Stat(root)
	if err != nil {
		return fmt.Errorf("go-template renderer: template root %q: %w", root, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("go-template renderer: template root %q is not a directory", root)
	}
	if err := os.MkdirAll(stagingDir, 0o755); err != nil {
		return fmt.Errorf("go-template renderer: create staging: %w", err)
	}
	data := buildTemplateData(spec.Answers)
	err = filepath.WalkDir(root, func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		dst := filepath.Join(stagingDir, rel)
		if d.IsDir() {
			return os.MkdirAll(dst, 0o755)
		}
		raw, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		// Static-scaffold template (.template): strip the suffix from the staged
		// name and copy the body verbatim — it is NOT a Go text/template (its
		// body carries {{ALL_CAPS}} sentinel placeholders, not Go template
		// syntax). The exec bit is derived from the suffix-stripped name.
		name := d.Name()
		if fileIsStaticTemplate(name) {
			stripped := strings.TrimSuffix(dst, TemplateAltSuffix)
			// .template body is verbatim EXCEPT the canonical harness-token
			// sentinels ({{PROJECT_SLUG}} etc.), which are resolved so a static
			// scaffold file ships token-free.
			return os.WriteFile(stripped, SubstituteHarnessTokens(raw, spec.Answers), renderWriteMode(stripped))
		}
		// Render only files marked with the Go text/template suffix; strip the
		// suffix from the staged name. Everything else is copied byte-for-byte,
		// then run through the harness-token pass so plain (non-.tmpl) files
		// ship token-free too (a shipped command carries no literal
		// {{COORDINATOR_DIR}}). The pass is a tight allowlist, so a literal
		// "{{ looks }}" that is not a canonical harness token survives untouched.
		if !strings.HasSuffix(name, suffix) {
			return os.WriteFile(dst, SubstituteHarnessTokens(raw, spec.Answers), renderWriteMode(name))
		}
		// No missingkey option: Go's default map-index behavior matches the
		// Jinja semantics the corpus conditionals rely on — an absent feature
		// flag ({{ if .features.backlog }} with backlog unset) is FALSY, so the
		// block is excluded, rather than erroring. This is the behavior Slice-3
		// backlog conditionals expect. buildTemplateData always seeds the
		// "features" map so .features itself resolves.
		t, err := texttemplate.New(name).Parse(string(raw))
		if err != nil {
			return fmt.Errorf("go-template renderer: parse %q: %w", rel, err)
		}
		var out bytes.Buffer
		if err := t.Execute(&out, data); err != nil {
			return fmt.Errorf("go-template renderer: execute %q: %w", rel, err)
		}
		stripped := strings.TrimSuffix(dst, suffix)
		// Resolve harness tokens on the EXECUTED output too, so a .tmpl that
		// emits a {{COORDINATOR_DIR}} sentinel (or whose body carries one outside
		// a Go-template action) ships resolved. No-op when no sentinel is present.
		return os.WriteFile(stripped, SubstituteHarnessTokens(out.Bytes(), spec.Answers), renderWriteMode(stripped))
	})
	if err != nil {
		return fmt.Errorf("go-template renderer: walk %q: %w", root, err)
	}
	return nil
}

// buildTemplateData turns the flat string answers into the data context handed
// to text/template. It:
//   - nests dotted keys (e.g. "features.backlog") into nested maps so
//     {{ .features.backlog }} resolves through the map chain;
//   - coerces "true"/"false" to Go bools so {{ if .features.backlog }} evaluates
//     the boolean (a non-empty "false" string would otherwise be truthy);
//   - derives project_slug from project_name (lower + spaces->dashes) when not
//     supplied, mirroring the former copier.yml default;
//   - ensures a "features" map always exists (even empty) so {{ if .features.X
//     }} resolves the map and treats an unset flag as falsy (the Jinja semantics
//     the corpus conditionals rely on), rather than erroring on a missing key.
//
// The renderer uses Go's default map-index behavior (no missingkey option): an
// absent feature flag is falsy in a conditional (block excluded), matching how
// Copier/Jinja handle undefined answers.
func buildTemplateData(answers map[string]string) map[string]any {
	root := map[string]any{}
	for k, v := range answers {
		setNested(root, k, coerce(v))
	}
	if _, ok := root["features"]; !ok {
		root["features"] = map[string]any{}
	}
	if _, ok := root["project_slug"]; !ok {
		if name, ok := root["project_name"].(string); ok {
			root["project_slug"] = slugify(name)
		}
	}
	return root
}

// setNested writes val into m at the (possibly dotted) key, creating
// intermediate maps as needed.
func setNested(m map[string]any, key string, val any) {
	parts := strings.Split(key, ".")
	cur := m
	for i, p := range parts {
		if i == len(parts)-1 {
			cur[p] = val
			return
		}
		next, ok := cur[p].(map[string]any)
		if !ok {
			next = map[string]any{}
			cur[p] = next
		}
		cur = next
	}
}

// coerce maps the canonical string forms of booleans to Go bools so template
// conditionals evaluate truthiness correctly. All other values stay strings.
func coerce(v string) any {
	switch v {
	case "true":
		return true
	case "false":
		return false
	default:
		return v
	}
}

// slugify mirrors the former copier.yml project_slug default
// ({{ project_name | lower | replace(' ', '-') }}): lowercase and spaces->dashes.
func slugify(name string) string {
	s := strings.ToLower(name)
	s = strings.ReplaceAll(s, " ", "-")
	return s
}

// Harness token sentinels. These are the canonical {{UPPER_TOKEN}} placeholders
// the corpus uses in NON-.tmpl (and .template) static-scaffold files. They are
// Jinja-style placeholders ({{PROJECT_NAME}}), NOT Go text/template actions
// ({{ .project_name }}). The Go template engine only ever parses .tmpl files, so
// without an explicit substitution pass these sentinels would ship LITERAL into
// a consumer's tree (e.g. .local/{{COORDINATOR_DIR}}/ inside a shipped command).
//
// These are the ONLY tokens the renderer resolves. Every other {{...}} form in
// the corpus survives untouched — including Go-template/LaTeX-style literals such
// as "{{ looks }}", and soft fill-in-by-hand placeholders such as
// {{MISSION_SUMMARY}}, {{DB_USER}}, or {{DEMO_VPS_FINGERPRINT}} that have no
// canonical install answer. Keeping this allowlist tight is what lets the pass
// run on every file (verbatim + .template + executed-.tmpl output) without ever
// corrupting a literal "{{" that is meant to stay literal.
const (
	harnessTokenProjectName    = "{{PROJECT_NAME}}"
	harnessTokenProjectSlug    = "{{PROJECT_SLUG}}"
	harnessTokenCoordinatorDir = "{{COORDINATOR_DIR}}"
)

// SubstituteHarnessTokens resolves the canonical {{UPPER_TOKEN}} sentinels in
// body using the install answers. It is called on the FINAL bytes of every
// staged CORE file (after .tmpl execution for templated files, raw for verbatim
// and .template files) by both GoTemplateRenderer and EmbedFSRenderer, so a
// shipped command or script can never carry a literal {{COORDINATOR_DIR}}. The
// overlay package applies the SAME pass to overlay-unit files and prompt-
// extension snippet bodies (internal/overlay RenderUnits / InjectExtensionSnippets)
// so a consumer's overlay output is token-free too — consistent with core.
//
// Resolution rules:
//   - {{PROJECT_NAME}}    -> answers["project_name"] verbatim (no case change).
//   - {{COORDINATOR_DIR}} -> answers["coordinator_dir"], defaulting to
//     "coordinator" when unset (the README default install dir is
//     .local/coordinator/).
//   - {{PROJECT_SLUG}}    -> CASE-AWARE: when the sentinel is immediately
//     followed by '_' (SCREAMING_SNAKE identifier context, e.g.
//     {{PROJECT_SLUG}}_JWT_SECRET) it substitutes the UPPER-CASED slug;
//     otherwise (paths, container names, image prefixes — e.g.
//     {{PROJECT_SLUG}}-dev-1) it substitutes the lower-case slug. This mirrors
//     the README token-vocabulary rule. project_slug defaults to
//     slugify(project_name) when unset (matching buildTemplateData).
//
// The fast path returns body unchanged when none of the three sentinels appear,
// so token-free files (the common case) pay only three bytes.Contains scans and
// no allocation. A nil/empty answers map is likewise a no-op (every resolved
// value is the empty string, but the fast path short-circuits before any
// replacement when no sentinel is present), which lets callers that have no
// answers (e.g. unit tests) pass nil and get verbatim bytes back. It does NOT
// touch FixtureRenderer (the test double copies verbatim by contract).
func SubstituteHarnessTokens(body []byte, answers map[string]string) []byte {
	if !bodyContainsHarnessToken(body) {
		return body
	}
	projectName := answers["project_name"]
	coordinatorDir := answers["coordinator_dir"]
	if coordinatorDir == "" {
		coordinatorDir = "coordinator"
	}
	slug := answers["project_slug"]
	if slug == "" {
		slug = slugify(projectName)
	}
	body = bytes.ReplaceAll(body, []byte(harnessTokenProjectName), []byte(projectName))
	body = bytes.ReplaceAll(body, []byte(harnessTokenCoordinatorDir), []byte(coordinatorDir))
	body = substituteProjectSlugSentinel(body, slug)
	return body
}

// bodyContainsHarnessToken is the fast-path guard for SubstituteHarnessTokens:
// true only when body carries at least one of the three canonical sentinels.
func bodyContainsHarnessToken(body []byte) bool {
	return bytes.Contains(body, []byte(harnessTokenProjectName)) ||
		bytes.Contains(body, []byte(harnessTokenProjectSlug)) ||
		bytes.Contains(body, []byte(harnessTokenCoordinatorDir))
}

// substituteProjectSlugSentinel performs the case-aware {{PROJECT_SLUG}}
// substitution. For each occurrence it inspects the byte immediately AFTER the
// closing }}: '_' -> UPPER-CASED slug (SCREAMING_SNAKE identifier context),
// else lower-cased slug (path / container / image context). An occurrence at
// EOF (no trailing byte) defaults to lower, matching the path/default context.
func substituteProjectSlugSentinel(body []byte, slug string) []byte {
	tok := []byte(harnessTokenProjectSlug)
	lower := strings.ToLower(slug)
	upper := strings.ToUpper(slug)
	var out bytes.Buffer
	out.Grow(len(body))
	i := 0
	for i < len(body) {
		j := bytes.Index(body[i:], tok)
		if j < 0 {
			out.Write(body[i:])
			break
		}
		start := i + j
		out.Write(body[i:start]) // bytes before this occurrence
		after := start + len(tok)
		if after < len(body) && body[after] == '_' {
			out.WriteString(upper)
		} else {
			out.WriteString(lower)
		}
		i = after
	}
	return out.Bytes()
}

// FixtureRenderer is a FAITHFUL TEST simulation of the render-into-staging step.
// It copies a local template directory tree verbatim into the staging directory
// with NO template substitution. It is the fast stand-in used to prove the seam
// logic (per-class apply + schema reconcile + lineage) without exercising the
// template engine — useful for unit tests that care about the seam, not the
// render mechanism.
//
// IMPORTANT: only the RENDER step is substituted. Everything downstream of
// Render (classification, per-class apply, schema reconcile, lineage) is real
// and identical for GoTemplateRenderer and FixtureRenderer — the seam calls
// Render then operates on the staged tree. For tests that need actual template
// substitution, use GoTemplateRenderer (covered by renderer_test.go).
type FixtureRenderer struct {
	// TemplateRoot is the local directory copied verbatim into staging.
	TemplateRoot string
}

// Name implements Renderer.
func (FixtureRenderer) Name() string { return "fixture-test-renderer" }

// Render copies the TemplateRoot tree into stagingDir, faithful to the production
// render's NAMING contract: a file whose name ends in TemplateSuffix (.tmpl) is
// staged under the suffix-stripped name (the real GoTemplateRenderer parses +
// renders it; the fixture copies the raw bytes). The content is byte-identical to
// the source (no substitution) — suffix stripping is a naming convention, not
// template execution — which is exactly what the seam's per-class apply needs: a
// candidate set of platform defaults to merge against the live tree, keyed by the
// LIVE (suffix-stripped) path the ownership classifier resolves.
func (r FixtureRenderer) Render(stagingDir string, spec RenderSpec) error {
	root := r.TemplateRoot
	if root == "" {
		root = spec.TemplateSource
	}
	info, err := os.Stat(root)
	if err != nil {
		return fmt.Errorf("fixture renderer: template root %q: %w", root, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("fixture renderer: template root %q is not a directory", root)
	}
	if err := os.MkdirAll(stagingDir, 0o755); err != nil {
		return fmt.Errorf("fixture renderer: create staging: %w", err)
	}
	err = filepath.WalkDir(root, func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		// Stage under the suffix-stripped name, matching the production renderer
		// (both .tmpl and .template suffixes are stripped). The exec bit is
		// derived from the suffix-stripped name, matching production.
		relStripped := stripTemplateSuffix(rel)
		dst := filepath.Join(stagingDir, relStripped)
		if d.IsDir() {
			return os.MkdirAll(dst, 0o755)
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(dst, data, renderWriteMode(relStripped))
	})
	if err != nil {
		return fmt.Errorf("fixture renderer: copy tree: %w", err)
	}
	return nil
}
