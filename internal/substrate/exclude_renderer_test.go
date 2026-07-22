package substrate

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	corpus "github.com/vhqtvn/vh-agent-harness" // root package: embed vars corpus.CoreFS etc.
)

// TestEmbedFSRenderer_ExcludeLivePaths proves ExcludeLivePaths filters a plain
// (non-.tmpl) file out of the staged tree entirely — the file is never read,
// never templated, never written. This is the core Slice-2 contract: an
// unselected capability's owned file does not render.
func TestEmbedFSRenderer_ExcludeLivePath(t *testing.T) {
	src := fstest.MapFS{
		"keep.md":                  &fstest.MapFile{Data: []byte("keep me\n")},
		".opencode/agents/skip.md": &fstest.MapFile{Data: []byte("skip me\n")},
	}
	staging := t.TempDir()
	r := EmbedFSRenderer{Source: src}
	if err := r.Render(staging, RenderSpec{
		Answers: map[string]string{},
		ExcludeLivePaths: map[string]bool{
			".opencode/agents/skip.md": true,
		},
	}); err != nil {
		t.Fatalf("render: %v", err)
	}
	// The kept file is staged.
	if _, err := os.Stat(filepath.Join(staging, "keep.md")); err != nil {
		t.Fatalf("keep.md should be staged: %v", err)
	}
	// The excluded file is NOT staged, and the directory is not created.
	if _, err := os.Stat(filepath.Join(staging, ".opencode", "agents", "skip.md")); !os.IsNotExist(err) {
		t.Fatalf("excluded skip.md must NOT be staged; stat err=%v", err)
	}
}

// TestEmbedFSRenderer_ExcludeLivePathTemplated proves the exclude check works
// on templated (.tmpl) sources too: the declared LIVE path is the
// suffix-stripped form, and the renderer strips the suffix before checking.
func TestEmbedFSRenderer_ExcludeLivePathTemplated(t *testing.T) {
	src := fstest.MapFS{
		".opencode/agents/skip.md.tmpl": &fstest.MapFile{
			Data: []byte("name={{ .project_name }}\n"),
		},
	}
	staging := t.TempDir()
	r := EmbedFSRenderer{Source: src}
	if err := r.Render(staging, RenderSpec{
		Answers: map[string]string{"project_name": "X"},
		ExcludeLivePaths: map[string]bool{
			// Declared as the LIVE (suffix-stripped) form.
			".opencode/agents/skip.md": true,
		},
	}); err != nil {
		t.Fatalf("render: %v", err)
	}
	if _, err := os.Stat(filepath.Join(staging, ".opencode", "agents", "skip.md")); !os.IsNotExist(err) {
		t.Fatalf("excluded .tmpl source must NOT be staged; stat err=%v", err)
	}
}

// TestEmbedFSRenderer_NoExcludeRendersAll proves nil/empty ExcludeLivePaths is
// the default unconditional walk — no file is filtered.
func TestEmbedFSRenderer_NoExcludeRendersAll(t *testing.T) {
	src := fstest.MapFS{
		"a.md": &fstest.MapFile{Data: []byte("a\n")},
		"b.md": &fstest.MapFile{Data: []byte("b\n")},
	}
	staging := t.TempDir()
	r := EmbedFSRenderer{Source: src}
	if err := r.Render(staging, RenderSpec{Answers: map[string]string{}}); err != nil {
		t.Fatalf("render: %v", err)
	}
	for _, rel := range []string{"a.md", "b.md"} {
		if _, err := os.Stat(filepath.Join(staging, rel)); err != nil {
			t.Fatalf("%s should be staged with no exclude set: %v", rel, err)
		}
	}
}

// TestEmbedFSRenderer_ExcludeDirectoryKept proves that excluding a file in a
// directory does not remove the directory walk — sibling files in the same
// directory still render.
func TestEmbedFSRenderer_ExcludeDirectoryKept(t *testing.T) {
	src := fstest.MapFS{
		".opencode/agents/keep.md":  &fstest.MapFile{Data: []byte("keep\n")},
		".opencode/agents/skip.md":  &fstest.MapFile{Data: []byte("skip\n")},
		".opencode/agents/skip2.md": &fstest.MapFile{Data: []byte("skip2\n")},
	}
	staging := t.TempDir()
	r := EmbedFSRenderer{Source: src}
	if err := r.Render(staging, RenderSpec{
		Answers: map[string]string{},
		ExcludeLivePaths: map[string]bool{
			".opencode/agents/skip.md":  true,
			".opencode/agents/skip2.md": true,
		},
	}); err != nil {
		t.Fatalf("render: %v", err)
	}
	// Sibling kept.
	if _, err := os.Stat(filepath.Join(staging, ".opencode", "agents", "keep.md")); err != nil {
		t.Fatalf("sibling keep.md should be staged: %v", err)
	}
	// Two excluded.
	for _, rel := range []string{"skip.md", "skip2.md"} {
		if _, err := os.Stat(filepath.Join(staging, ".opencode", "agents", rel)); !os.IsNotExist(err) {
			t.Fatalf("excluded %s must NOT be staged; stat err=%v", rel, err)
		}
	}
}

// TestGoTemplateRenderer_ExcludeLivePath proves the filesystem-backed renderer
// applies the same ExcludeLivePaths filter as the embed renderer (parity).
func TestGoTemplateRenderer_ExcludeLivePath(t *testing.T) {
	root := t.TempDir()
	writeCorpusFile(t, root, "keep.md", "keep\n")
	writeCorpusFile(t, root, ".opencode/agents/skip.md", "skip\n")
	writeCorpusFile(t, root, ".opencode/agents/keep.md", "keep-sibling\n")

	staging := t.TempDir()
	r := GoTemplateRenderer{TemplateRoot: root}
	if err := r.Render(staging, RenderSpec{
		Answers: map[string]string{},
		ExcludeLivePaths: map[string]bool{
			".opencode/agents/skip.md": true,
		},
	}); err != nil {
		t.Fatalf("render: %v", err)
	}
	if got := readStaged(t, staging, "keep.md"); got != "keep\n" {
		t.Fatalf("keep.md content mismatch: %q", got)
	}
	if got := readStaged(t, staging, filepath.FromSlash(".opencode/agents/keep.md")); got != "keep-sibling\n" {
		t.Fatalf("sibling keep.md content mismatch: %q", got)
	}
	if _, err := os.Stat(filepath.Join(staging, ".opencode", "agents", "skip.md")); !os.IsNotExist(err) {
		t.Fatalf("excluded skip.md must NOT be staged; stat err=%v", err)
	}
}

// TestGoTemplateRenderer_ExcludeLivePathTemplated proves the filesystem
// renderer's exclude also works on .tmpl sources (suffix-stripped check).
func TestGoTemplateRenderer_ExcludeLivePathTemplated(t *testing.T) {
	root := t.TempDir()
	writeCorpusFile(t, root, ".opencode/agents/skip.md.tmpl", "name={{ .project_name }}\n")

	staging := t.TempDir()
	r := GoTemplateRenderer{TemplateRoot: root}
	if err := r.Render(staging, RenderSpec{
		Answers: map[string]string{"project_name": "X"},
		ExcludeLivePaths: map[string]bool{
			".opencode/agents/skip.md": true, // LIVE form
		},
	}); err != nil {
		t.Fatalf("render: %v", err)
	}
	if _, err := os.Stat(filepath.Join(staging, ".opencode", "agents", "skip.md")); !os.IsNotExist(err) {
		t.Fatalf("excluded .tmpl source must NOT be staged; stat err=%v", err)
	}
}

// TestRendererExcludeParity proves the embed and filesystem renderers produce
// IDENTICAL staged live-path sets for both the selected and unselected cases.
// This is the core parity guarantee for Slice 2: the renderer used by install
// (embed) and the renderer used by tmp-tools (filesystem) agree on what renders.
func TestRendererExcludeParity(t *testing.T) {
	// Build an identical source tree in both forms: fstest.MapFS (embed) and
	// a real temp dir (filesystem).
	mediaAgent := ".opencode/agents/media-perception.md"
	mediaSkill := ".opencode/skills/media-perception/SKILL.md"
	keepFile := ".opencode/agents/build.md"

	files := map[string]string{
		mediaAgent: "# media-perception\n",
		mediaSkill: "# skill\n",
		keepFile:   "# build\n",
	}

	// fstest.MapFS
	mapSrc := fstest.MapFS{}
	for p, body := range files {
		mapSrc[p] = &fstest.MapFile{Data: []byte(body)}
	}

	// Filesystem temp dir
	fsRoot := t.TempDir()
	for p, body := range files {
		writeCorpusFile(t, fsRoot, p, body)
	}

	excludeSets := []struct {
		name    string
		exclude map[string]bool
	}{
		{"selected (no exclude)", nil},
		{"unselected (exclude media)", map[string]bool{
			mediaAgent: true,
			mediaSkill: true,
		}},
	}

	for _, tc := range excludeSets {
		t.Run(tc.name, func(t *testing.T) {
			embedStaging := t.TempDir()
			embedR := EmbedFSRenderer{Source: mapSrc}
			if err := embedR.Render(embedStaging, RenderSpec{
				Answers:          map[string]string{},
				ExcludeLivePaths: tc.exclude,
			}); err != nil {
				t.Fatalf("embed render: %v", err)
			}
			fsStaging := t.TempDir()
			fsR := GoTemplateRenderer{TemplateRoot: fsRoot}
			if err := fsR.Render(fsStaging, RenderSpec{
				Answers:          map[string]string{},
				ExcludeLivePaths: tc.exclude,
			}); err != nil {
				t.Fatalf("filesystem render: %v", err)
			}
			embedPaths := walkStagedSet(t, embedStaging)
			fsPaths := walkStagedSet(t, fsStaging)
			// Compare the sets.
			if len(embedPaths) != len(fsPaths) {
				t.Fatalf("path count mismatch: embed=%d fs=%d\nembed=%v\nfs=%v",
					len(embedPaths), len(fsPaths), embedPaths, fsPaths)
			}
			for p := range embedPaths {
				if !fsPaths[p] {
					t.Errorf("path %q staged by embed but not filesystem", p)
				}
			}
			for p := range fsPaths {
				if !embedPaths[p] {
					t.Errorf("path %q staged by filesystem but not embed", p)
				}
			}
		})
	}
}

// TestRendererExcludeRealCoreCorpus proves the exclude behavior against the
// REAL embedded templates/core corpus: with the media-perception live paths
// excluded, neither file nor its directory renders; with no exclude, both do.
func TestRendererExcludeRealCoreCorpus(t *testing.T) {
	coreSub, err := fs.Sub(corpus.CoreFS, corpus.CoreDir)
	if err != nil {
		t.Fatalf("fs.Sub(core): %v", err)
	}
	mediaAgent := ".opencode/agents/media-perception.md"
	mediaSkill := ".opencode/skills/media-perception/SKILL.md"

	t.Run("selected (both present)", func(t *testing.T) {
		staging := t.TempDir()
		r := EmbedFSRenderer{Source: coreSub}
		if err := r.Render(staging, RenderSpec{Answers: map[string]string{}}); err != nil {
			t.Fatalf("render: %v", err)
		}
		for _, rel := range []string{mediaAgent, mediaSkill} {
			if _, err := os.Stat(filepath.Join(staging, filepath.FromSlash(rel))); err != nil {
				t.Fatalf("selected: %s should be staged: %v", rel, err)
			}
		}
	})

	t.Run("unselected (both absent)", func(t *testing.T) {
		staging := t.TempDir()
		r := EmbedFSRenderer{Source: coreSub}
		if err := r.Render(staging, RenderSpec{
			Answers: map[string]string{},
			ExcludeLivePaths: map[string]bool{
				mediaAgent: true,
				mediaSkill: true,
			},
		}); err != nil {
			t.Fatalf("render: %v", err)
		}
		for _, rel := range []string{mediaAgent, mediaSkill} {
			p := filepath.Join(staging, filepath.FromSlash(rel))
			if _, err := os.Stat(p); !os.IsNotExist(err) {
				t.Fatalf("unselected: %s must NOT be staged; stat err=%v", rel, err)
			}
		}
	})
}

// walkStagedSet returns the set of forward-slash rel paths under staging.
func walkStagedSet(t *testing.T, staging string) map[string]bool {
	t.Helper()
	out := map[string]bool{}
	_ = filepath.Walk(staging, func(p string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(staging, p)
		out[filepath.ToSlash(rel)] = true
		return nil
	})
	return out
}
