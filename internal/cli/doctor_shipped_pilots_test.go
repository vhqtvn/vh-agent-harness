package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vhqtvn/vh-agent-harness/internal/lineage"
)

// doctor_shipped_pilots_test.go tests doctor check #23 (shipped-pilots): the
// enablement-state reporting for each shipped default-on overlay pilot and the
// gate-2 advisory-orphan rule (opt-out NEVER makes a repo unhealthy).

// seedMinimalLineage writes a minimal valid S1 lineage.yml so isSeamInstalled
// returns true for the test target. The lineage carries only lineage_version;
// all other S1 fields are optional and zero-value for test purposes.
func seedMinimalLineage(t *testing.T, target string) {
	t.Helper()
	dir := filepath.Join(target, lineage.DirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	content := "lineage_version: \"1\"\n"
	if err := os.WriteFile(filepath.Join(dir, lineage.FileName), []byte(content), 0o644); err != nil {
		t.Fatalf("write lineage: %v", err)
	}
}

// writeSkillFile simulates a prior render by creating a skill file under
// .opencode/skills/<name>/SKILL.md in the target.
func writeSkillFile(t *testing.T, target, skillName string) {
	t.Helper()
	skillDir := filepath.Join(target, ".opencode", "skills", skillName)
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir skill dir: %v", err)
	}
	skillFile := filepath.Join(skillDir, "SKILL.md")
	if err := os.WriteFile(skillFile, []byte("# stale skill from prior render\n"), 0o644); err != nil {
		t.Fatalf("write skill file: %v", err)
	}
}

// TestShippedPilots_AllEnabledByDefault: a bare minimal profile with no
// explicit feature overrides or overlays: entries → all 3 pilots report
// enabled-by-platform-default, tierPass.
func TestShippedPilots_AllEnabledByDefault(t *testing.T) {
	target := t.TempDir()
	seedMinimalLineage(t, target)
	writeProfile(t, target, "profile: minimal\noverlays: []\n")

	r := checkShippedPilots(target)

	if r.tier != tierPass {
		t.Fatalf("tier: got %q, want %q (all-default-on must PASS)", r.tier, tierPass)
	}
	for _, pilot := range []string{"formal-verification-pilot", "resolve-first-pilot", "contract-invariant-audit-pilot"} {
		want := pilot + ": " + pilotStateEnabledDefault
		if !strings.Contains(r.detail, want) {
			t.Errorf("detail missing %q; got: %s", want, r.detail)
		}
	}
}

// TestShippedPilots_DisabledByConsumerOverride: features: <key>: false on one
// pilot → that pilot reports disabled-by-consumer-override; the others stay
// enabled-by-platform-default.
func TestShippedPilots_DisabledByConsumerOverride(t *testing.T) {
	target := t.TempDir()
	seedMinimalLineage(t, target)
	writeProfile(t, target, ""+
		"profile: minimal\n"+
		"features:\n"+
		"  resolve-first-pilot: false\n"+
		"overlays: []\n")

	r := checkShippedPilots(target)

	// Opt-out is intentional consumer choice → still PASS (not even WARN).
	if r.tier != tierPass {
		t.Fatalf("tier: got %q, want %q (opt-out alone must not WARN/FAIL)", r.tier, tierPass)
	}
	if !strings.Contains(r.detail, "resolve-first-pilot: "+pilotStateDisabledOverride) {
		t.Errorf("expected resolve-first-pilot disabled; got: %s", r.detail)
	}
	if !strings.Contains(r.detail, "formal-verification-pilot: "+pilotStateEnabledDefault) {
		t.Errorf("expected formal-verification-pilot still enabled; got: %s", r.detail)
	}
}

// TestShippedPilots_ExplicitOverlaySurvivesOptOut: an explicit overlays: entry
// for a pilot whose feature is false → the pilot reports explicitly-selected
// (the explicit entry survives the false; features:false is NOT a global veto).
func TestShippedPilots_ExplicitOverlaySurvivesOptOut(t *testing.T) {
	target := t.TempDir()
	seedMinimalLineage(t, target)
	writeProfile(t, target, ""+
		"profile: minimal\n"+
		"features:\n"+
		"  formal-verification-pilot: false\n"+
		"overlays:\n"+
		"  - formal-verification-pilot\n")

	r := checkShippedPilots(target)

	if r.tier != tierPass {
		t.Fatalf("tier: got %q, want %q", r.tier, tierPass)
	}
	if !strings.Contains(r.detail, "formal-verification-pilot: "+pilotStateExplicit) {
		t.Errorf("expected formal-verification-pilot explicitly-selected; got: %s", r.detail)
	}
}

// TestShippedPilots_AdvisoryOrphanNeverFails (GATE-2 TEST): a disabled pilot
// whose skill files exist on disk (from a prior render) is an ADVISORY orphan.
// The check reports tierInfo (NOT tierFail) — opting out must NEVER make a repo
// unhealthy.
func TestShippedPilots_AdvisoryOrphanNeverFails(t *testing.T) {
	target := t.TempDir()
	seedMinimalLineage(t, target)
	writeProfile(t, target, ""+
		"profile: minimal\n"+
		"features:\n"+
		"  formal-verification-pilot: false\n"+
		"overlays: []\n")

	// Simulate a prior render: create the pilot's skill file on disk.
	writeSkillFile(t, target, "formal-verification")

	r := checkShippedPilots(target)

	// THE GATE-2 ASSERTION: advisory orphan is tierInfo, NEVER tierFail.
	if r.tier == tierFail {
		t.Fatalf("GATE-2 VIOLATION: advisory orphan produced tierFail — opting out must NEVER make a repo unhealthy; got: %s", r.detail)
	}
	if r.tier != tierInfo {
		t.Fatalf("tier: got %q, want %q (advisory orphan should be INFO)", r.tier, tierInfo)
	}
	if !strings.Contains(r.detail, "advisory orphan") {
		t.Errorf("expected advisory orphan mention; got: %s", r.detail)
	}
}

// TestShippedPilots_AdvisoryOrphanClearedWhenSkillRemoved: when the stale skill
// files are removed, the disabled pilot no longer reports an advisory orphan.
func TestShippedPilots_AdvisoryOrphanClearedWhenSkillRemoved(t *testing.T) {
	target := t.TempDir()
	seedMinimalLineage(t, target)
	writeProfile(t, target, ""+
		"profile: minimal\n"+
		"features:\n"+
		"  formal-verification-pilot: false\n"+
		"overlays: []\n")

	// No stale files on disk → no orphan.

	r := checkShippedPilots(target)

	if r.tier != tierPass {
		t.Fatalf("tier: got %q, want %q (disabled pilot with no stale files should PASS)", r.tier, tierPass)
	}
	if strings.Contains(r.detail, "advisory orphan") {
		t.Errorf("should not report orphan when no stale files; got: %s", r.detail)
	}
}

// TestShippedPilots_SkipWhenNotSeamInstalled: a greenfield/adoptable repo
// (no lineage) → SKIP.
func TestShippedPilots_SkipWhenNotSeamInstalled(t *testing.T) {
	target := t.TempDir()
	// No lineage written → isSeamInstalled returns false.

	r := checkShippedPilots(target)

	if r.tier != tierSkip {
		t.Fatalf("tier: got %q, want %q (greenfield should SKIP)", r.tier, tierSkip)
	}
}
