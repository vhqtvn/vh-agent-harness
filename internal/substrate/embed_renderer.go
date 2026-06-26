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

// EmbedFSRenderer renders a template corpus backed by an embed.FS (or any
// fs.FS) into a staging directory. It mirrors GoTemplateRenderer's .tmpl
// convention exactly — files whose name ends in TemplateSuffix are parsed and
// executed through text/template (suffix stripped from the staged name); every
// other file is copied verbatim — but reads from an in-memory fs.FS rather than
// the local filesystem.
//
// Why a second renderer: GoTemplateRenderer is filesystem-only (os.Stat /
// filepath.WalkDir / os.ReadFile) and the canonical end-to-end proof
// (tmp-tools/render-apply) runs it against an on-disk templates/core. The CLI,
// however, ships a self-contained binary: the corpus is embedded, and install
// must work from any CWD without a templates/core checkout on disk. Rather than
// rewrite GoTemplateRenderer (and its tests + the proof), this additive renderer
// gives the CLI an embed-backed render with identical .tmpl semantics.
//
// It does NOT change GoTemplateRenderer or FixtureRenderer. All three satisfy
// the Renderer interface and feed the same downstream seam (classify → per-class
// apply → schema reconcile → lineage).
type EmbedFSRenderer struct {
	// Source is the fs.FS rooted at the corpus root (the embed.FS sub-tree via
	// fs.Sub, or any fs.FS). Files are read relative to this root.
	Source fs.FS
	// TemplateSuffix overrides the default TemplateSuffix when non-empty.
	TemplateSuffix string
}

// Name implements Renderer.
func (EmbedFSRenderer) Name() string { return "embed-fs" }

// Render walks the Source fs.FS, renders *.tmpl files through text/template into
// stagingDir (suffix stripped from the staged name), and copies every other
// file verbatim. Directories are mirrored. The walk is deterministic
// (fs.WalkDir visits lexically) so identical inputs stage identical trees.
//
// The template-data construction (buildTemplateData) and the missingkey
// semantics are identical to GoTemplateRenderer, so a template that renders one
// way under GoTemplateRenderer renders the same way here.
func (r EmbedFSRenderer) Render(stagingDir string, spec RenderSpec) error {
	if r.Source == nil {
		return fmt.Errorf("embed-fs renderer: Source fs.FS is nil")
	}
	suffix := r.TemplateSuffix
	if suffix == "" {
		suffix = TemplateSuffix
	}
	if err := os.MkdirAll(stagingDir, 0o755); err != nil {
		return fmt.Errorf("embed-fs renderer: create staging: %w", err)
	}
	data := buildTemplateData(spec.Answers)
	err := fs.WalkDir(r.Source, ".", func(rel string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		// rel is relative to the corpus root and uses forward slashes.
		dst := filepath.Join(stagingDir, filepath.FromSlash(rel))
		if d.IsDir() {
			return os.MkdirAll(dst, 0o755)
		}
		raw, err := fs.ReadFile(r.Source, rel)
		if err != nil {
			return fmt.Errorf("embed-fs renderer: read %q: %w", rel, err)
		}
		name := d.Name()
		// Static-scaffold template (.template): strip suffix, copy verbatim then
		// resolve the canonical harness-token sentinels ({{PROJECT_SLUG}} etc.)
		// so the static scaffold ships token-free. Exec bit is derived from the
		// suffix-stripped name.
		if fileIsStaticTemplate(name) {
			stripped := strings.TrimSuffix(dst, TemplateAltSuffix)
			return os.WriteFile(stripped, SubstituteHarnessTokens(raw, spec.Answers), renderWriteMode(stripped))
		}
		if !strings.HasSuffix(name, suffix) {
			// Plain (non-.tmpl) file: copied byte-for-byte then run through the
			// harness-token pass (tight allowlist) so a shipped command/script
			// carries no literal {{COORDINATOR_DIR}}.
			return os.WriteFile(dst, SubstituteHarnessTokens(raw, spec.Answers), renderWriteMode(name))
		}
		// Same missingkey semantics as GoTemplateRenderer (default map-index:
		// absent key is falsy in a conditional, not an error).
		t, err := texttemplate.New(name).Parse(string(raw))
		if err != nil {
			return fmt.Errorf("embed-fs renderer: parse %q: %w", rel, err)
		}
		var out bytes.Buffer
		if err := t.Execute(&out, data); err != nil {
			return fmt.Errorf("embed-fs renderer: execute %q: %w", rel, err)
		}
		stripped := strings.TrimSuffix(dst, suffix)
		return os.WriteFile(stripped, SubstituteHarnessTokens(out.Bytes(), spec.Answers), renderWriteMode(stripped))
	})
	if err != nil {
		return fmt.Errorf("embed-fs renderer: walk: %w", err)
	}
	return nil
}
