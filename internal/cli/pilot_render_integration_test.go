package cli

// pilot_render_integration_test.go proves the load-bearing behavioral crux:
// a consumer with no explicit overlays: entry gains the 3 shipped pilot skills
// via the render path (pack name → actual staged skill file), and can disable
// them via features: <key>: false. This resolves the DEFER finding from Commit
// B's review (F1-B: end-to-end render path not tested) and is the behavioral-
// closure evidence for the opt-out pilot distribution slice.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vhqtvn/vh-agent-harness/internal/substrate"
)

// renderPilotStaging renders the core corpus + active overlays into a fresh
// staging dir using the SAME pipeline seamApply/doctor use. It returns the
// overlay file paths that were actually rendered (the proof the pilot packs
// flowed through the shared render staging as overlay_extension).
func renderPilotStaging(t *testing.T, root string) []string {
	t.Helper()
	staging := t.TempDir()
	sub, err := coreSubFSImpl()
	if err != nil {
		t.Fatalf("coreSubFSImpl: %v", err)
	}
	renderer := substrate.EmbedFSRenderer{Source: sub}
	answers := mergeRenderAnswers(installRenderAnswers(root), readProfileAnswers(root))
	overlayFiles, _, _, err := renderSeamStaging(staging, renderer, answers, root)
	if err != nil {
		t.Fatalf("renderSeamStaging: %v", err)
	}
	return overlayFiles
}

// TestPilotRender_DefaultOnRendersAllThree (THE CRUX): a bare minimal profile
// with NO explicit overlays: entries renders all 3 pilot skill files through
// the shared render staging. This is the behavioral proof that default-on
// distribution reaches the consumer's .opencode/skills/ tree.
func TestPilotRender_DefaultOnRendersAllThree(t *testing.T) {
	root := t.TempDir()
	seamInstallInto(t, root)
	// Overwrite the profile to a bare minimal (no explicit overlays, no
	// feature overrides → all 3 pilots default-on via reconciledFeatures).
	writeProfile(t, root, "profile: minimal\noverlays: []\n")

	overlayFiles := renderPilotStaging(t, root)

	for _, skillPath := range []string{
		".opencode/skills/formal-verification/SKILL.md",
		".opencode/skills/resolve-first/SKILL.md",
		".opencode/skills/contract-invariant-audit/SKILL.md",
	} {
		found := false
		for _, f := range overlayFiles {
			if f == skillPath {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected pilot skill %q in rendered overlay files; got %v", skillPath, overlayFiles)
		}
	}
}

// TestPilotRender_OptOutFalseDropsOne: features: <key>: false removes only that
// pilot's skill from the render output (the others remain).
func TestPilotRender_OptOutFalseDropsOne(t *testing.T) {
	root := t.TempDir()
	seamInstallInto(t, root)
	writeProfile(t, root, ""+
		"profile: minimal\n"+
		"features:\n"+
		"  resolve-first-pilot: false\n"+
		"overlays: []\n")

	overlayFiles := renderPilotStaging(t, root)

	// resolve-first must be ABSENT.
	for _, f := range overlayFiles {
		if strings.Contains(f, "skills/resolve-first/") {
			t.Errorf("resolve-first should be DROPPED by features:false; found %q in %v", f, overlayFiles)
		}
	}
	// The other two must still be present.
	for _, skillPath := range []string{
		".opencode/skills/formal-verification/SKILL.md",
		".opencode/skills/contract-invariant-audit/SKILL.md",
	} {
		found := false
		for _, f := range overlayFiles {
			if f == skillPath {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected pilot skill %q still rendered; got %v", skillPath, overlayFiles)
		}
	}
}

// TestPilotRender_ExplicitOverlaySurvivesOptOut: an explicit overlays: entry
// re-adds a pilot even when features: <key>: false (NOT a global veto).
func TestPilotRender_ExplicitOverlaySurvivesOptOut(t *testing.T) {
	root := t.TempDir()
	seamInstallInto(t, root)
	writeProfile(t, root, ""+
		"profile: minimal\n"+
		"features:\n"+
		"  formal-verification-pilot: false\n"+
		"overlays:\n"+
		"  - formal-verification-pilot\n")

	overlayFiles := renderPilotStaging(t, root)

	// formal-verification must be PRESENT (explicit overlay survives the false).
	found := false
	for _, f := range overlayFiles {
		if f == ".opencode/skills/formal-verification/SKILL.md" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("explicit overlays: entry must survive features:false; formal-verification skill missing from %v", overlayFiles)
	}
}

// TestPilotRender_StagedFilesExistOnDisk: the rendered overlay files actually
// land on disk in the staging dir (not just returned as strings). This closes
// the pack-name → skill-file-on-disk gap.
func TestPilotRender_StagedFilesExistOnDisk(t *testing.T) {
	root := t.TempDir()
	seamInstallInto(t, root)
	writeProfile(t, root, "profile: minimal\noverlays: []\n")

	staging := t.TempDir()
	sub, err := coreSubFSImpl()
	if err != nil {
		t.Fatalf("coreSubFSImpl: %v", err)
	}
	renderer := substrate.EmbedFSRenderer{Source: sub}
	answers := mergeRenderAnswers(installRenderAnswers(root), readProfileAnswers(root))
	overlayFiles, _, _, err := renderSeamStaging(staging, renderer, answers, root)
	if err != nil {
		t.Fatalf("renderSeamStaging: %v", err)
	}

	for _, f := range overlayFiles {
		if !strings.HasPrefix(f, ".opencode/skills/") {
			continue // only check skill files for this assertion
		}
		livePath := filepath.Join(staging, filepath.FromSlash(f))
		if _, err := os.Stat(livePath); err != nil {
			t.Errorf("staged overlay file %q does not exist on disk at %s: %v", f, livePath, err)
		}
	}
}
