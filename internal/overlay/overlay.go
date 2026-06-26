package overlay

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"

	corpus "github.com/vhqtvn/vh-agent-harness"
	"github.com/vhqtvn/vh-agent-harness/internal/substrate"
)

// Merge-content / catalog file names that live at a pack root and are NOT
// rendered as unit files (they are deep-merged / materialized elsewhere
// instead).
const (
	appendFileName         = "opencode-append.jsonc"
	snippetFileName        = "callable-graph-snippet.md"
	permissionPackFileName = "permission-pack.jsonc"
)

// Live (staged) paths that the merge-content files target. These mirror the
// paths the core corpus renders, so the overlay merges operate on files that
// already exist in staging.
const (
	opencodeTargetRel = "opencode.jsonc"
	callableGraphRel  = ".opencode/docs/agents/callable-graph.md"
)

// opencodePrefix is the live subtree overlay unit files render under. A pack's
// agents/<x>.md becomes <staging>/.opencode/agents/<x>.md, mirroring how the
// core corpus lays out the .opencode tree.
const opencodePrefix = ".opencode/"

// Pack is an opened overlay pack: a name plus its embedded fs.FS rooted at the
// pack directory. Unit files (agents/, skills/, commands/) mirror the .opencode
// subtree and render verbatim under <staging>/.opencode/. The two merge-content
// files at the pack root are consumed by MergeAppend / AppendCallableGraph.
type Pack struct {
	Name string
	FS   fs.FS
}

// KnownPacks lists the immediate pack directories under the embedded overlays
// tree, sorted. Each directory name is a selectable pack (referenced from
// vh-harness-profile.yml: overlays:[...]). Unknown names in a profile are skipped
// at apply time rather than aborting the install.
//
// As of the 2026-06-25 pre-publish clearance the shipped overlays tree carries
// NO packs (web-overlay was relocated to a non-shipped adoption reference), so
// KnownPacks returns an empty slice. The mechanism is retained as the forward
// path: drop a pack directory under templates/overlays/ and it is listed here
// with no code change.
func KnownPacks() ([]string, error) {
	sub, err := fs.Sub(corpus.OverlaysFS, corpus.OverlaysDir)
	if err != nil {
		return nil, fmt.Errorf("overlay: list packs: %w", err)
	}
	entries, err := fs.ReadDir(sub, ".")
	if err != nil {
		return nil, fmt.Errorf("overlay: list packs: %w", err)
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	return names, nil
}

// ProjectOverlaysSubdir is the project-local directory under which a consuming
// repo ships its OWN overlay packs: <target>/.vh-agent-harness/overlays/<name>/.
// This keeps the binary domain-free — every project supplies its own packs
// rather than embedding them into the shared binary. A project-local pack
// SHADOWS an embedded pack of the same name (project-wins), so a fork can
// override a shipped pack without rebuilding.
const ProjectOverlaysSubdir = ".vh-agent-harness/overlays"

// OpenPack opens a named overlay pack from the embedded overlays FS. Returns an
// error wrapping fs.ErrNotExist if the pack directory is absent. Prefer
// OpenPackFor, which also resolves project-local packs.
func OpenPack(name string) (*Pack, error) {
	packPath := path.Join(corpus.OverlaysDir, name)
	sub, err := fs.Sub(corpus.OverlaysFS, packPath)
	if err != nil {
		return nil, fmt.Errorf("overlay: open pack %q: %w", name, err)
	}
	// Verify the directory exists and is readable.
	if _, err := fs.ReadDir(sub, "."); err != nil {
		return nil, fmt.Errorf("overlay: open pack %q: %w", name, err)
	}
	return &Pack{Name: name, FS: sub}, nil
}

// OpenPackFor opens a named overlay pack, resolving a PROJECT-LOCAL pack at
// <target>/.vh-agent-harness/overlays/<name>/ FIRST, then falling back to the
// embedded overlays FS. This lets each consuming project ship its own packs
// (the binary stays domain-free). target may be "" to force embedded-only.
// Returns an error wrapping fs.ErrNotExist when neither source has the pack.
func OpenPackFor(target, name string) (*Pack, error) {
	if target != "" {
		dir := filepath.Join(target, filepath.FromSlash(ProjectOverlaysSubdir), name)
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			return &Pack{Name: name, FS: os.DirFS(dir)}, nil
		}
	}
	return OpenPack(name)
}

// isUnitFile reports whether a pack-relative path is a renderable unit (i.e. not
// one of the merge-content / catalog files that live at the pack root, and not a
// prompt-extension snippet whose name matches <base>.extend.<slot>.<ext>).
// Extension snippets are render-time injection material (see ExtensionSnippets),
// never standalone units.
func isUnitFile(rel string) bool {
	if rel == appendFileName || rel == snippetFileName || rel == permissionPackFileName {
		return false
	}
	if isExtensionSnippet(rel) {
		return false
	}
	return true
}

// RenderUnits copies every unit file in the pack into
// <staging>/.opencode/<pack-rel>, creating directories as needed. Each unit
// body is run through substrate.SubstituteHarnessTokens — the SAME tight-
// allowlist {{PROJECT_NAME}} / {{PROJECT_SLUG}} / {{COORDINATOR_DIR}} pass the
// core renderers apply to every staged core file (see renderer.go and
// embed_renderer.go) — so a consumer's overlay-unit output ships token-free,
// consistent with core. Soft fill-in-by-hand placeholders
// ({{TOKEN}}, {{DEMO_VPS_FINGERPRINT}}, ...) are NOT in the allowlist and stay
// literal by design. When answers is nil/empty (or the body carries no
// sentinel), the pass is a no-op fast path, so callers without answers get
// verbatim bytes — the behavior every token-free pack (and the unit tests
// passing nil) relies on. Returns the sorted list of LIVE .opencode-relative
// paths that were rendered, so the classifier can mark them overlay_extension.
func (p *Pack) RenderUnits(staging string, answers map[string]string) ([]string, error) {
	var rendered []string
	err := fs.WalkDir(p.FS, ".", func(rel string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() || !isUnitFile(rel) {
			return nil
		}
		srcFile, err := p.FS.Open(rel)
		if err != nil {
			return err
		}
		defer srcFile.Close()
		data, err := io.ReadAll(srcFile)
		if err != nil {
			return err
		}
		liveRel := opencodePrefix + rel
		dst := filepath.Join(staging, filepath.FromSlash(liveRel))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		// Resolve the 3 canonical identity tokens before writing, mirroring the
		// core-render write site. No-op when answers is empty or the body carries
		// no sentinel (fast path returns the input slice unchanged).
		data = substrate.SubstituteHarnessTokens(data, answers)
		if err := os.WriteFile(dst, data, 0o644); err != nil {
			return err
		}
		rendered = append(rendered, liveRel)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("overlay %s: render units: %w", p.Name, err)
	}
	sort.Strings(rendered)
	return rendered, nil
}

// MergeAppend deep-merges the pack's opencode-append.jsonc (if present) into the
// staged opencode.jsonc. No-op if the pack ships no append file. The staged
// opencode.jsonc must already exist (the core corpus renders it).
func (p *Pack) MergeAppend(staging string) error {
	appendBytes, err := fs.ReadFile(p.FS, appendFileName)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil // pack contributes no opencode entries (e.g. a skills-only pack)
		}
		return fmt.Errorf("overlay %s: read append: %w", p.Name, err)
	}
	targetPath := filepath.Join(staging, opencodeTargetRel)
	base, err := os.ReadFile(targetPath)
	if err != nil {
		return fmt.Errorf("overlay %s: staged opencode.jsonc missing: %w", p.Name, err)
	}
	merged, err := MergeJSONC(base, appendBytes)
	if err != nil {
		return fmt.Errorf("overlay %s: merge opencode.jsonc: %w", p.Name, err)
	}
	if err := os.WriteFile(targetPath, merged, 0o644); err != nil {
		return fmt.Errorf("overlay %s: write merged opencode.jsonc: %w", p.Name, err)
	}
	return nil
}

// AppendCallableGraph appends the pack's callable-graph-snippet.md (if present)
// to the staged callable-graph.md. No-op if the pack ships no snippet, or if the
// core corpus rendered no callable-graph.md (the append then has nowhere to go).
func (p *Pack) AppendCallableGraph(staging string) error {
	snippetBytes, err := fs.ReadFile(p.FS, snippetFileName)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil // pack contributes no callable-graph additions
		}
		return fmt.Errorf("overlay %s: read snippet: %w", p.Name, err)
	}
	targetPath := filepath.Join(staging, filepath.FromSlash(callableGraphRel))
	f, err := os.OpenFile(targetPath, os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil // core has no callable-graph.md; nothing to append to
		}
		return fmt.Errorf("overlay %s: open callable-graph.md: %w", p.Name, err)
	}
	defer f.Close()
	// Separate the snippet from the core body with a blank line pair.
	if _, err := f.Write(append([]byte("\n\n"), snippetBytes...)); err != nil {
		return fmt.Errorf("overlay %s: append callable-graph snippet: %w", p.Name, err)
	}
	return nil
}

// permissionPackRel is the live (staged) subtree under which each active pack's
// self-describing permission descriptor is materialized. The core permission
// script (update-opencode-config.js) reads this directory DYNAMICALLY (by
// directory listing) to resolve the active agent roster, so the harness core
// never hardcodes any overlay pack by name.
const permissionPackRel = ".opencode/sys-scripts/permission-packs/"

// MaterializePermissionPack copies the pack's permission-pack.jsonc (if present)
// into <staging>/.opencode/sys-scripts/permission-packs/<pack>.jsonc and returns
// the LIVE .opencode-relative path that was written, so the seam classifier can
// mark it overlay_extension. Returns "" (no-op) if the pack ships no descriptor
// (e.g. a pack that contributes skills only and no permission-relevant agents).
func (p *Pack) MaterializePermissionPack(staging string) (string, error) {
	srcBytes, err := fs.ReadFile(p.FS, permissionPackFileName)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", nil // pack contributes no permission descriptor
		}
		return "", fmt.Errorf("overlay %s: read permission-pack: %w", p.Name, err)
	}
	liveRel := permissionPackRel + p.Name + ".jsonc"
	dst := filepath.Join(staging, filepath.FromSlash(liveRel))
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return "", fmt.Errorf("overlay %s: materialize permission-pack: %w", p.Name, err)
	}
	if err := os.WriteFile(dst, srcBytes, 0o644); err != nil {
		return "", fmt.Errorf("overlay %s: materialize permission-pack: %w", p.Name, err)
	}
	return liveRel, nil
}
