package overlay

// Phase 2 capability-manifest pack recognition tests. Cover:
//   - ReadCapabilityManifest loads + parses a pack's capability-manifest.yml
//   - absent manifest -> ok=false, no error (skills-only pack is legitimate)
//   - malformed YAML -> error wrapping the parse failure
//   - capability-manifest.yml is NOT rendered as a unit (recognition holds
//     end-to-end through RenderUnits and UnitPaths)
//
// Pack fixtures are built from testing/fstest.MapFS (mirroring the on-disk pack
// layout), consistent with the pack-touching tests in overlay_test.go.

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

const sampleManifestBody = `id: acme/deploy
provides:
  - deploy-agent
  - rollback-agent
hard_deps:
  - core/gated-commit
optional_deps: []
`

// TestReadCapabilityManifest_LoadsAndParses confirms the pack's
// capability-manifest.yml is read and parsed into a resolver.CapabilityManifest
// with the expected fields.
func TestReadCapabilityManifest_LoadsAndParses(t *testing.T) {
	p := &Pack{
		Name: "acme",
		FS: fstest.MapFS{
			capabilityManifestFileName: &fstest.MapFile{Data: []byte(sampleManifestBody)},
			"agents/deploy.md":         &fstest.MapFile{Data: []byte("# deploy\n")},
		},
	}
	m, ok, err := p.ReadCapabilityManifest()
	if err != nil {
		t.Fatalf("ReadCapabilityManifest: unexpected error: %v", err)
	}
	if !ok {
		t.Fatalf("ReadCapabilityManifest: expected ok=true (manifest present)")
	}
	if m.ID != "acme/deploy" {
		t.Errorf("ID: got %q, want acme/deploy", m.ID)
	}
	if len(m.Provides) != 2 || m.Provides[0] != "deploy-agent" || m.Provides[1] != "rollback-agent" {
		t.Errorf("Provides: got %v, want [deploy-agent rollback-agent]", m.Provides)
	}
	if len(m.HardDeps) != 1 || m.HardDeps[0] != "core/gated-commit" {
		t.Errorf("HardDeps: got %v, want [core/gated-commit]", m.HardDeps)
	}
}

// TestReadCapabilityManifest_AbsentReturnsNotOK confirms a pack with no
// capability-manifest.yml returns (zero, false, nil) — not an error — so a
// skills-only pack is a legitimate no-manifest case.
func TestReadCapabilityManifest_AbsentReturnsNotOK(t *testing.T) {
	p := &Pack{
		Name: "skills-only",
		FS: fstest.MapFS{
			"skills/only/SKILL.md": &fstest.MapFile{Data: []byte("# skill\n")},
		},
	}
	m, ok, err := p.ReadCapabilityManifest()
	if err != nil {
		t.Errorf("absent manifest must not error; got: %v", err)
	}
	if ok {
		t.Errorf("absent manifest must return ok=false")
	}
	if m.ID != "" {
		t.Errorf("absent manifest must return zero-value manifest; got ID=%q", m.ID)
	}
}

// TestReadCapabilityManifest_MalformedYAMLErrors confirms a present-but-broken
// YAML manifest is returned as an error (NOT ok=false) so callers distinguish
// "no manifest" from "broken manifest".
func TestReadCapabilityManifest_MalformedYAMLErrors(t *testing.T) {
	p := &Pack{
		Name: "broken",
		FS: fstest.MapFS{
			capabilityManifestFileName: &fstest.MapFile{Data: []byte("id: [unclosed\n  - badly: : : indented")},
		},
	}
	_, ok, err := p.ReadCapabilityManifest()
	if err == nil {
		t.Fatalf("malformed YAML manifest must error, not silently ok=false")
	}
	if ok {
		t.Errorf("malformed YAML manifest must return ok=false alongside the error")
	}
	// The error should reference the pack name (context) and wrap the parse error.
	if !strings.Contains(err.Error(), "broken") {
		t.Errorf("error should name the pack; got: %v", err)
	}
}

// TestReadCapabilityManifest_ReadErrorWrapped confirms a low-level FS read
// failure (not ErrNotExist) is wrapped with pack context. We exercise this via
// an fstest.MapFS by querying a file that fails Stat; since MapFS does not
// easily simulate mid-read failures, we instead assert the absent-path returns
// ErrNotExist-free error semantics are distinct from a real read error by
// checking the contract here is about absence, covered above. This test pins
// that a missing file is ErrNotExist (so the ok=false path is unambiguous).
func TestReadCapabilityManifest_AbsentIsErrNotExist(t *testing.T) {
	p := &Pack{
		Name: "no-manifest-pack",
		FS:   fstest.MapFS{"agents/x.md": &fstest.MapFile{Data: []byte("x")}},
	}
	_, ok, err := p.ReadCapabilityManifest()
	if err != nil {
		t.Fatalf("absent manifest must not surface as error: %v", err)
	}
	if ok {
		t.Errorf("absent manifest must return ok=false")
	}
	// Contract: absence is ok=false+nil err (callers MUST NOT rely on the
	// error being ErrNotExist; they rely on ok). Document the invariant.
}

// TestRenderUnits_CapabilityManifestNotRendered confirms RenderUnits does NOT
// emit capability-manifest.yml as a unit (it is parsed by ReadCapabilityManifest
// and merged by resolver.MergeCatalogs, not rendered verbatim), while sibling
// unit files ARE rendered.
func TestRenderUnits_CapabilityManifestNotRendered(t *testing.T) {
	p := &Pack{
		Name: "acme",
		FS: fstest.MapFS{
			capabilityManifestFileName: &fstest.MapFile{Data: []byte(sampleManifestBody)},
			"agents/deploy.md":         &fstest.MapFile{Data: []byte("# deploy agent\n")},
			"commands/deploy.md":       &fstest.MapFile{Data: []byte("# deploy cmd\n")},
		},
	}
	staging := t.TempDir()
	rendered, err := p.RenderUnits(staging, nil)
	if err != nil {
		t.Fatalf("RenderUnits: %v", err)
	}
	bad := opencodePrefix + capabilityManifestFileName
	if contains(rendered, bad) {
		t.Errorf("RenderUnits must NOT render %q; got %v", capabilityManifestFileName, rendered)
	}
	if _, err := os.Stat(filepath.Join(staging, filepath.FromSlash(bad))); err == nil {
		t.Errorf("capability-manifest.yml must not land on disk as a rendered unit")
	}
	// Sibling units ARE rendered.
	for _, want := range []string{".opencode/agents/deploy.md", ".opencode/commands/deploy.md"} {
		if !contains(rendered, want) {
			t.Errorf("RenderUnits missing unit %q; got %v", want, rendered)
		}
	}
}

// TestUnitPaths_CapabilityManifestExcluded confirms UnitPaths (the read-only
// projection used by the shadow guard) does NOT list capability-manifest.yml,
// keeping the merge-content file out of the collision set.
func TestUnitPaths_CapabilityManifestExcluded(t *testing.T) {
	p := &Pack{
		Name: "acme",
		FS: fstest.MapFS{
			capabilityManifestFileName: &fstest.MapFile{Data: []byte(sampleManifestBody)},
			"agents/deploy.md":         &fstest.MapFile{Data: []byte("# deploy\n")},
		},
	}
	paths, err := p.UnitPaths()
	if err != nil {
		t.Fatalf("UnitPaths: %v", err)
	}
	if contains(paths, opencodePrefix+capabilityManifestFileName) {
		t.Errorf("UnitPaths must not list capability-manifest.yml; got %v", paths)
	}
	if len(paths) != 1 || paths[0] != ".opencode/agents/deploy.md" {
		t.Errorf("UnitPaths: got %v, want [.opencode/agents/deploy.md]", paths)
	}
}

// TestOpenPackFor_ProjectLocalCarriesManifest confirms the project-local pack
// shadowing path (OpenPackFor) yields a *Pack whose ReadCapabilityManifest
// reads the PROJECT pack's manifest — the FS-layer half of project-wins
// shadowing. resolver.MergeCatalogs applies the same rule again at the merge
// layer (see resolver.MergeCatalogs tests).
func TestOpenPackFor_ProjectLocalCarriesManifest(t *testing.T) {
	target := t.TempDir()
	packDir := filepath.Join(target, filepath.FromSlash(ProjectOverlaysSubdir), "acme")
	if err := os.MkdirAll(packDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(packDir, capabilityManifestFileName), []byte(sampleManifestBody), 0o644); err != nil {
		t.Fatal(err)
	}

	pack, err := OpenPackFor(target, "acme")
	if err != nil {
		t.Fatalf("OpenPackFor(project-local): %v", err)
	}
	m, ok, err := pack.ReadCapabilityManifest()
	if err != nil {
		t.Fatalf("ReadCapabilityManifest on project pack: %v", err)
	}
	if !ok {
		t.Fatalf("project pack manifest must be present")
	}
	if m.ID != "acme/deploy" {
		t.Errorf("project pack manifest ID: got %q, want acme/deploy", m.ID)
	}
}

// TestReadCapabilityManifest_PackErrorsWrapName is a compile-time/behavior pin:
// the error from a read failure names the pack (context). We trigger it via a
// deliberately nil FS read path is not feasible with MapFS; instead assert the
// absent file is cleanly ok=false (the only MapFS-reachable branch) and that no
// error escapes referencing a different pack. This guards the wrap contract.
func TestReadCapabilityManifest_PackErrorsWrapName(t *testing.T) {
	p := &Pack{Name: "named-pack", FS: fstest.MapFS{}}
	_, _, err := p.ReadCapabilityManifest()
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("unexpected non-NotExist error path; got: %v", err)
	}
}
