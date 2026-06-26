package cli

// This file is the regression guard for the SilenceErrors UX fix (Part 3).
// update/doctor run with SilenceErrors:true so Cobra prints no "Error:" line.
// Before the fix, a rejected ownership downgrade (a genuine *runtime* error,
// not errSilent) was returned straight to root.Execute -> os.Exit(1) with NO
// printed reason. The fix prints the error to stderr exactly once before the
// non-zero exit. This test pins that the downgrade reason (path, from->to
// classes, raise-only guidance, the future reviewed-downgrade hint) is visible
// in the command output, and that the exit is still non-zero.

import (
	"os"
	"strings"
	"testing"

	"github.com/vhqtvn/vh-agent-harness/internal/lineage"
	"github.com/vhqtvn/vh-agent-harness/internal/ownership"
)

// TestRunUpdate_DowngradeRejectionPrintsReasonToStderr installs a fresh tree,
// then writes a harness-ownership.yml override that lowers vh-harness-profile.yml
// from platform_armed -> platform_managed (a raise-only violation). runUpdate
// must (a) abort with a *ownership.DowngradeError, and (b) print the human-
// readable reason to stderr (captured here via newOutCmd's shared buffer).
func TestRunUpdate_DowngradeRejectionPrintsReasonToStderr(t *testing.T) {
	root := t.TempDir()
	seamInstallInto(t, root)

	// Lower vh-harness-profile.yml (platform_armed in core) to platform_managed.
	// Resolve rejects this under the raise-only rule and returns DowngradeError.
	ownershipOverride := []byte("overrides:\n  .vh-agent-harness/vh-harness-profile.yml:\n    class: " +
		string(ownership.ClassPlatformManaged) + "\n    reason: \"test: lower armed profile\"\n")
	writeHarnessFile(t, root, "harness-ownership.yml", ownershipOverride)

	out, err := seamUpdateOut(t, root)

	// (a) The apply aborts with a downgrade rejection (non-zero exit preserved).
	if err == nil {
		t.Fatal("runUpdate: want error for downgrade override, got nil")
	}
	if !ownership.IsDowngradeError(err) {
		t.Errorf("runUpdate error is not a DowngradeError; got %v", err)
	}

	// (b) The human-readable reason now reaches the command output (stderr is
	// routed to the same buffer by newOutCmd). Before the fix this was empty on a
	// rejected downgrade.
	for _, want := range []string{
		"error:", // the prefixed stderr line
		".vh-agent-harness/vh-harness-profile.yml", // the rejected path
		"raise-only",                           // the rule name + guidance
		"vh-agent-harness ownership downgrade", // the future reviewed-downgrade hint
	} {
		if !strings.Contains(out, want) {
			t.Errorf("runUpdate output missing %q\n--- output ---\n%s", want, out)
		}
	}
}

// TestRunUpdate_DowngradeDoesNotMutateTree confirms the rejected downgrade is
// detected before any write touches the live tree: the staged opencode.jsonc
// must be absent (seamApply aborted at classify time, before substrate.Apply).
func TestRunUpdate_DowngradeDoesNotMutateTree(t *testing.T) {
	root := t.TempDir()
	seamInstallInto(t, root)
	ownershipOverride := []byte("overrides:\n  .vh-agent-harness/vh-harness-profile.yml:\n    class: " +
		string(ownership.ClassPlatformManaged) + "\n    reason: \"downgrade\"\n")
	writeHarnessFile(t, root, "harness-ownership.yml", ownershipOverride)

	if _, err := seamUpdateOut(t, root); err == nil {
		t.Fatal("runUpdate: want error for downgrade, got nil")
	}
	// No seam staging opencode.jsonc should have been written into the live tree.
	live := root + "/.opencode/opencode.jsonc"
	if fileExists(live) {
		t.Errorf("rejected downgrade must not write live opencode.jsonc; %s exists", live)
	}
}

// TestRunDoctor_UnhealthyStillUsesErrSilent confirms the SilenceErrors fix did
// not regress doctor's normal UNHEALTHY path: a poisoned lineage yields an
// UNHEALTHY report to stdout, but NO "error:" stderr line (doctor returns
// errSilent there, which reportRunErrToStderr skips). This is the regression
// guard that the fix only added output for GENUINE errors, not the errSilent
// problems>0 path that already prints its own report.
func TestRunDoctor_UnhealthyStillUsesErrSilent(t *testing.T) {
	root := t.TempDir()
	seamInstallInto(t, root)
	// Poison the lineage (canonical pattern from TestSeamDoctor_UnhealthyOnLineageAuthorityLeak).
	poisoned := []byte("lineage_version: \"1\"\ntemplate: {source: x}\n" +
		"copier: {version: \"\"}\nanswers: {digest: sha256:x}\n" +
		"render: {last_successful_update_id: x}\n" +
		"profile: minimal\nservices: {web: {}}\n")
	if err := os.WriteFile(lineage.FilePath(root), poisoned, 0o644); err != nil {
		t.Fatal(err)
	}

	out := seamDoctorOut(t, root)
	// UNHEALTHY report is present (doctor's own stdout output)...
	if !strings.Contains(out, "UNHEALTHY") {
		t.Errorf("doctor output missing UNHEALTHY report\n--- output ---\n%s", out)
	}
	// ...but NO stderr "error:" line is printed (errSilent path, pre-existing behavior).
	if strings.Contains(out, "error:") {
		t.Errorf("doctor UNHEALTHY path must stay silent on stderr (errSilent); got:\n%s", out)
	}
}

// writeHarnessFile writes body into <root>/.vh-agent-harness/<name>.
func writeHarnessFile(t *testing.T, root, name string, body []byte) {
	t.Helper()
	dir := root + "/.vh-agent-harness"
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir lineage dir: %v", err)
	}
	if err := os.WriteFile(dir+"/"+name, body, 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

// fileExists reports whether path exists on disk.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
