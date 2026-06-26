// Package manifest defines the on-disk harness manifest schema, provides
// read-only lookup of .opencode/harness-manifest.json from the current
// directory or any ancestor, and writes a freshly rendered manifest during
// install.
//
// Slice 1 introduced Find() + a minimal read struct. Slice 2 reshapes the
// struct to the canonical install-time schema (enabled_components as a flat
// string list, files as a path-keyed map with hash + class) and adds Write().
package manifest

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// DirName is the per-project directory that holds harness state.
const DirName = ".opencode"

// FileName is the manifest file name inside DirName.
const FileName = "harness-manifest.json"

// SchemaVersion is the manifest schema version written by this binary.
const SchemaVersion = "1"

// Ownership classes assigned to manifest.files entries. Slice 2 marks every
// embedded corpus file ClassManaged. Slice 6 names the two classes that the
// render/upgrade/uninstall lifecycle verbs discriminate on:
//
//   - managed              : rendered from the corpus; overwritten by
//     render/upgrade and removed by uninstall.
//   - generated-from-config: derived from config at install/enable time;
//     treated like managed by the lifecycle verbs.
//   - project-owned        : owned by the project. NEVER auto-rendered,
//     auto-overwritten, or auto-removed; uninstall
//     preserves it unless --force is given.
//
// Slice 5.1 CONVERGES this parallel vocabulary onto the armed ownership lattice
// (internal/ownership). The seam/lineage install path (install/update/doctor)
// already speaks the six armed classes exclusively; the legacy manifest model
// (render/upgrade/uninstall/preflight/enable/disable) now recognizes the same
// lattice names alongside the legacy names. IsRenderable honors both vocabularies
// so a manifest written with either set discriminates consistently. The legacy
// string values are UNCHANGED (old manifests keep parsing); the lattice consts
// below are the additive, converged surface.
const (
	ClassManaged             = "managed"
	ClassGeneratedFromConfig = "generated-from-config"
	ClassProjectOwned        = "project-owned"

	// Armed-lattice classes (the converged vocabulary; mirror
	// internal/ownership.Class). The seam path uses these exclusively.
	ClassPlatformManagedV   = "platform_managed"
	ClassPlatformArmedV     = "platform_armed"
	ClassOverlayExtensionV  = "overlay_extension"
	ClassProjectOwnedV      = "project_owned"
	ClassExternalGeneratedV = "external_generated"
	ClassLocalOnlyV         = "local_only"
)

// IsRenderable reports whether a file class is harness-rendered and therefore
// refreshed by `render`/`upgrade` and removed by `uninstall`. project-owned
// files are never auto-rendered or auto-removed by the lifecycle verbs. It is
// the single discriminator used by render/upgrade/uninstall so they agree on
// which manifest.files entries they may touch.
//
// Slice 5.1: the discriminator honors BOTH the legacy vocabulary
// (managed / generated-from-config) AND the converged armed lattice
// (platform_managed / platform_armed / overlay_extension are renderable;
// project_owned / external_generated / local_only are not). This is the
// manifest-side analogue of ownership.IsPlatformOverwritable +
// IsMutableByPlatform: renderable classes are exactly those a platform render
// may touch, and protected/off-path classes are exactly those it may not.
func IsRenderable(class string) bool {
	switch class {
	case ClassManaged, ClassGeneratedFromConfig,
		ClassPlatformManagedV, ClassPlatformArmedV, ClassOverlayExtensionV:
		return true
	}
	return false
}

// DefaultEnabledComponents is the Slice 2 default component set written into a
// fresh manifest. Full component toggling arrives in Slice 3.
var DefaultEnabledComponents = []string{"plugin:shell-guard"}

// Manifest models the full harness-manifest.json document. The JSON shape is
// stable across read and write:
//
//	{
//	  "schema_version": "1",
//	  "harness_version": "0.1.0-dev (slice-1)",
//	  "project": { "name": "...", "slug": "..." },
//	  "runtime": { "backend": "docker_compose", "fallback": "" },
//	  "enabled_components": ["plugin:shell-guard"],
//	  "render_tokens": { "project_name": "...", "project_slug": "...",
//	                     "coordinator_dir": "...", "demo_vps_fingerprint": "" },
//	  "files": {
//	    ".opencode/scripts/state-lib.js": { "hash": "sha256:...", "class": "managed" }
//	  }
//	}
//
// The render_tokens object is a Slice 3 ADDITIVE extension (omitempty, so older
// manifests without it still parse). It records the four installer tokens used
// at install time so `enable` can re-render a component's files with byte-for-byte
// identical substitution ("4 sentinels same as install"). Without it, enabling a
// component whose files contain {{COORDINATOR_DIR}} (e.g. agent:coordination)
// after a non-default --coordinator-dir install would silently produce wrong
// content. The settled fields above are unchanged.
type Manifest struct {
	SchemaVersion     string          `json:"schema_version"`
	HarnessVersion    string          `json:"harness_version"`
	Project           Project         `json:"project"`
	Runtime           Runtime         `json:"runtime"`
	EnabledComponents []string        `json:"enabled_components"`
	RenderTokens      RenderTokens    `json:"render_tokens,omitempty"`
	Files             map[string]File `json:"files"`
}

// RenderTokens records the installer substitution values used at install time.
// It is persisted so later re-renders (enable, upgrade) reproduce the install
// byte-for-byte. ProjectName/ProjectSlug mirror Project.Name/ProjectSlug for
// rendering convenience.
type RenderTokens struct {
	ProjectName        string `json:"project_name"`
	ProjectSlug        string `json:"project_slug"`
	CoordinatorDir     string `json:"coordinator_dir"`
	DemoVPSFingerprint string `json:"demo_vps_fingerprint"`
}

// Project is the human + machine identity of the harness installation target.
type Project struct {
	Name string `json:"name"`
	Slug string `json:"slug"`
}

// Runtime declares which execution backend the harness drives plus an optional
// fallback. Slice 2 recorded only Backend/Fallback. Slice 4 ADDITIVELY extends
// the block with optional backend-specific fields (ComposeFile, DefaultService)
// so docker_compose can resolve its compose file and default exec service.
//
// Both new fields are `omitempty`; older Slice 2 manifests (without them) still
// parse with zero-value defaults resolved by the runtime layer. schema_version
// stays "1" — no migration required.
type Runtime struct {
	Backend        string `json:"backend"`
	Fallback       string `json:"fallback"`
	ComposeFile    string `json:"compose_file,omitempty"`
	DefaultService string `json:"default_service,omitempty"`
}

// File is one rendered file managed by the harness. The path is the map key in
// Manifest.Files; each entry carries the rendered content hash and an ownership
// class.
type File struct {
	Hash  string `json:"hash"`
	Class string `json:"class"`
}

// New returns a Manifest prefilled with the canonical schema version, an empty
// file map, and the Slice 2 default enabled components. Callers fill in
// HarnessVersion, Project, Runtime, and Files.
func New() *Manifest {
	return &Manifest{
		SchemaVersion:     SchemaVersion,
		EnabledComponents: append([]string(nil), DefaultEnabledComponents...),
		Files:             make(map[string]File),
	}
}

// FilePath returns the absolute path to the manifest inside a target directory.
// It does not check for existence.
func FilePath(targetDir string) string {
	return filepath.Join(targetDir, DirName, FileName)
}

// Write serializes the manifest as indented JSON to DirName/FileName under
// targetDir, creating the directory as needed. JSON object keys (including the
// Files map) are emitted in sorted order by encoding/json, so the output is
// deterministic across idempotent re-installs.
func (m *Manifest) Write(targetDir string) error {
	if err := os.MkdirAll(filepath.Join(targetDir, DirName), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(FilePath(targetDir), data, 0o644)
}

// Find walks upward from startDir looking for .opencode/harness-manifest.json.
//
// It returns the resolved absolute path and the parsed manifest when found.
// When no manifest exists between startDir and the filesystem root it returns
// ("", nil, nil). A present-but-unreadable or malformed manifest is returned
// as an error so callers can distinguish "absent" from "broken".
func Find(startDir string) (path string, m *Manifest, err error) {
	dir, err := filepath.Abs(startDir)
	if err != nil {
		return "", nil, err
	}
	for {
		candidate := filepath.Join(dir, DirName, FileName)
		if info, statErr := os.Stat(candidate); statErr == nil && !info.IsDir() {
			data, readErr := os.ReadFile(candidate)
			if readErr != nil {
				return candidate, nil, readErr
			}
			var parsed Manifest
			if jsonErr := json.Unmarshal(data, &parsed); jsonErr != nil {
				return candidate, nil, fmt.Errorf("parse %s: %w", candidate, jsonErr)
			}
			return candidate, &parsed, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", nil, nil // reached filesystem root
		}
		dir = parent
	}
}

// --- Component toggle helpers (Slice 3) -------------------------------------

// IsEnabled reports whether id is in EnabledComponents.
func (m *Manifest) IsEnabled(id string) bool {
	for _, e := range m.EnabledComponents {
		if e == id {
			return true
		}
	}
	return false
}

// EnableComponent appends id to EnabledComponents (preserving order, no
// duplicates) if it is not already present. It returns true when the manifest
// was mutated, false when id was already enabled.
func (m *Manifest) EnableComponent(id string) bool {
	if m.IsEnabled(id) {
		return false
	}
	m.EnabledComponents = append(m.EnabledComponents, id)
	return true
}

// DisableComponent removes id from EnabledComponents, preserving the order of
// the remaining entries. It returns true when the manifest was mutated, false
// when id was not enabled.
func (m *Manifest) DisableComponent(id string) bool {
	out := m.EnabledComponents[:0]
	changed := false
	for _, e := range m.EnabledComponents {
		if e == id {
			changed = true
			continue
		}
		out = append(out, e)
	}
	m.EnabledComponents = out
	return changed
}

// SetFile records or replaces a managed-file entry.
func (m *Manifest) SetFile(relPath string, f File) {
	if m.Files == nil {
		m.Files = make(map[string]File)
	}
	m.Files[relPath] = f
}

// RemoveFile deletes a managed-file entry. It is a no-op if the path is absent.
func (m *Manifest) RemoveFile(relPath string) {
	delete(m.Files, relPath)
}
