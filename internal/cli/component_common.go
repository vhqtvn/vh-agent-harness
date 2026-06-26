package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/vhqtvn/vh-agent-harness/internal/manifest"
)

// loadedManifest bundles a located runtime authority with the absolute
// project-root directory that anchors the shell-guard hook and lifecycle
// dispatcher. It is populated by EITHER the S4 run-shape (runtime verbs prefer
// S4) or the legacy manifest (the fallback + the authority for the management
// verbs). runtimeConfig is the backend-selection spec both authorities fill.
type loadedManifest struct {
	// path is the absolute path to the authority file (harness-manifest.json,
	// or "" when the authority is the S4 run-shape).
	path string
	// dir is the absolute project root containing .opencode/ (legacy) or
	// .vh-agent-harness/ (S4). It anchors the shell-guard hook + lifecycle
	// dispatcher.
	dir string
	// m is the parsed legacy manifest; nil when the authority is the S4
	// run-shape (the runtime verbs do not consult the manifest in that case).
	m *manifest.Manifest
	// runtimeConfig is the backend-selection spec, filled from S4 or the legacy
	// manifest. defaultBackendFor reads it via selectBackend.
	runtimeConfig runtimeConfig
	// source records which authority resolved ("run-shape" | "manifest"). Used
	// for diagnostics; not load-bearing for backend selection.
	source string
}

// loadManifest finds the governing manifest from the current working directory
// (walking up) and returns it. A missing manifest yields a clear error that
// points the user at `vh-agent-harness install`; a malformed one yields the parse error.
func loadManifest() (*loadedManifest, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	return loadManifestFrom(cwd)
}

// loadManifestFrom finds the governing manifest starting from startDir (walking
// up). It is the testable core of loadManifest and lets unit tests point at a
// temp project directory without changing the process cwd.
func loadManifestFrom(startDir string) (*loadedManifest, error) {
	path, m, err := manifest.Find(startDir)
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}
	if m == nil {
		return nil, fmt.Errorf("no harness manifest found in %s (or any parent) — run `vh-agent-harness install` first", startDir)
	}
	// path is <dir>/.opencode/harness-manifest.json -> project root is two Dir() up.
	dir := filepath.Dir(filepath.Dir(path))
	return &loadedManifest{
		path: path,
		dir:  dir,
		m:    m,
		runtimeConfig: runtimeConfig{
			backend:        m.Runtime.Backend,
			composeFile:    m.Runtime.ComposeFile,
			defaultService: m.Runtime.DefaultService,
			projectSlug:    m.Project.Slug,
		},
		source: "manifest",
	}, nil
}
