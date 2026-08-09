package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vhqtvn/vh-agent-harness/internal/lineage"
	"github.com/vhqtvn/vh-agent-harness/internal/originhash"
)

// These tests exercise the SEAM path of diff/uninstall/preflight (the default
// since install writes .vh-agent-harness/lineage.yml, not a legacy manifest).
// The legacy-manifest path stays covered by lifecycle_test.go.

const seamManagedProbe = ".opencode/agents/build.md"

// TestSeamDiff_CleanThenDrift: a fresh seam install diffs clean; editing a
// platform_managed file makes diff report it drifted and exit non-zero.
func TestSeamDiff_CleanThenDrift(t *testing.T) {
	root := t.TempDir()
	seamInstallInto(t, root)

	runWithCwd(t, root, func() {
		cmd, buf := newOutCmd()
		if err := runDiff(cmd, nil); err != nil {
			t.Fatalf("clean seam diff: want nil, got %v (out=%q)", err, buf.String())
		}
		if out := buf.String(); !strings.Contains(out, "0 drifted, 0 missing, 0 unexpected") {
			t.Errorf("clean diff summary unexpected: %q", out)
		}
	})

	corruptManaged(t, root, seamManagedProbe)
	runWithCwd(t, root, func() {
		cmd, buf := newOutCmd()
		err := runDiff(cmd, nil)
		if err == nil {
			t.Fatalf("drifted seam diff: want non-nil (errSilent), got nil (out=%q)", buf.String())
		}
		out := buf.String()
		if !strings.Contains(out, "drifted") || !strings.Contains(out, seamManagedProbe) {
			t.Errorf("diff want %s drifted, got %q", seamManagedProbe, out)
		}
	})
}

// TestSeamDiff_DetectsUnexpectedSkipsState: a non-corpus file under .opencode/
// is reported unexpected, but runtime-state subtrees are not flagged.
func TestSeamDiff_DetectsUnexpectedSkipsState(t *testing.T) {
	root := t.TempDir()
	seamInstallInto(t, root)

	rogue := filepath.Join(root, ".opencode", "agents", "rogue.md")
	if err := os.WriteFile(rogue, []byte("rogue\n"), 0o644); err != nil {
		t.Fatalf("write rogue: %v", err)
	}
	stateF := filepath.Join(root, ".opencode", "state", "session.json")
	if err := os.MkdirAll(filepath.Dir(stateF), 0o755); err != nil {
		t.Fatalf("mkdir state: %v", err)
	}
	if err := os.WriteFile(stateF, []byte("{}"), 0o644); err != nil {
		t.Fatalf("write state: %v", err)
	}

	runWithCwd(t, root, func() {
		cmd, buf := newOutCmd()
		err := runDiff(cmd, nil)
		if err == nil {
			t.Fatalf("unexpected file: want non-nil, got nil (out=%q)", buf.String())
		}
		out := buf.String()
		if !strings.Contains(out, "unexpected") || !strings.Contains(out, ".opencode/agents/rogue.md") {
			t.Errorf("diff want rogue.md unexpected, got %q", out)
		}
		if strings.Contains(out, "state/session.json") {
			t.Errorf("diff must NOT flag runtime state as unexpected, got %q", out)
		}
	})
}

// TestSeamUninstall_RemovesManagedPreservesOwnedAndLineageLast confirms the
// seam uninstall removes platform-controlled files, preserves a project_owned
// corpus file (.gitignore) and runtime state, and removes lineage last.
func TestSeamUninstall_RemovesManagedPreservesOwnedAndLineageLast(t *testing.T) {
	root := t.TempDir()
	seamInstallInto(t, root)

	managed := filepath.Join(root, filepath.FromSlash(seamManagedProbe))
	gitignore := filepath.Join(root, ".gitignore") // project_owned corpus seed
	if !pathExists(t, managed) {
		t.Fatalf("precondition: %s should exist after install", seamManagedProbe)
	}
	if !pathExists(t, lineage.FilePath(root)) {
		t.Fatalf("precondition: lineage should exist after install")
	}
	stateF := filepath.Join(root, ".opencode", "state", "s.json")
	os.MkdirAll(filepath.Dir(stateF), 0o755)
	os.WriteFile(stateF, []byte("{}"), 0o644)

	runWithCwd(t, root, func() {
		uninstallForce = false
		cmd, buf := newOutCmd()
		if err := runUninstall(cmd, nil); err != nil {
			t.Fatalf("seam uninstall: %v (out=%q)", err, buf.String())
		}
		if out := buf.String(); !strings.Contains(out, "leftover (intentionally preserved)") {
			t.Errorf("want leftover report, got %q", out)
		}
	})

	if pathExists(t, managed) {
		t.Errorf("managed %s still present after uninstall", seamManagedProbe)
	}
	if pathExists(t, lineage.FilePath(root)) {
		t.Errorf("lineage still present after uninstall (should be removed last)")
	}
	if !pathExists(t, gitignore) {
		t.Errorf("project_owned .gitignore removed (should be preserved without --force)")
	}
	if !pathExists(t, stateF) {
		t.Errorf("runtime state removed (should be preserved)")
	}
}

// TestSeamUninstall_RemovesOriginHashStoreSoReinstallReseeds is the regression
// lock for the origin-hash update sync lifecycle: install records the
// origin-hashes.json sidecar; uninstall MUST remove it (otherwise a later
// install reads stale entries and treats every previously-managed file as
// consumer-deleted → not re-seeded, breaking install→uninstall→install).
// Proves the full lifecycle: managed files are restored after a re-install.
func TestSeamUninstall_RemovesOriginHashStoreSoReinstallReseeds(t *testing.T) {
	root := t.TempDir()
	seamInstallInto(t, root)

	managed := filepath.Join(root, filepath.FromSlash(seamManagedProbe))
	// Precondition: install recorded the origin-hash sidecar.
	if !pathExists(t, originhash.FilePath(root)) {
		t.Fatalf("precondition: origin-hashes.json should exist after install")
	}

	// Uninstall.
	runWithCwd(t, root, func() {
		uninstallForce = false
		cmd, buf := newOutCmd()
		if err := runUninstall(cmd, nil); err != nil {
			t.Fatalf("seam uninstall: %v (out=%q)", err, buf.String())
		}
	})

	// The sidecar MUST be gone (else reinstall would treat managed files as
	// consumer-deleted and refuse to re-seed them).
	if pathExists(t, originhash.FilePath(root)) {
		t.Errorf("origin-hashes.json still present after uninstall (would block re-seed on reinstall)")
	}
	if pathExists(t, managed) {
		t.Errorf("managed %s still present after uninstall", seamManagedProbe)
	}

	// Re-install: managed files MUST be restored (clean-bootstrap semantics,
	// because the sidecar is gone so there is no stale origin to honor).
	seamInstallInto(t, root)
	if !pathExists(t, managed) {
		t.Errorf("managed %s NOT restored after reinstall (stale origin-hashes store survived uninstall?)", seamManagedProbe)
	}
	if !pathExists(t, originhash.FilePath(root)) {
		t.Errorf("origin-hashes.json should be re-recorded after reinstall")
	}
}

// TestSeamPreflight_PassThenDrift: fresh seam install passes preflight via the
// seam authorities; corrupting a managed file fails it on managed-drift.
func TestSeamPreflight_PassThenDrift(t *testing.T) {
	root := t.TempDir()
	seamInstallInto(t, root)

	runWithCwd(t, root, func() {
		cmd, buf := newOutCmd()
		if err := runPreflight(cmd, nil); err != nil {
			t.Fatalf("seam preflight pass: want nil, got %v (out=%q)", err, buf.String())
		}
		out := buf.String()
		if !strings.Contains(out, "result: PASS") {
			t.Errorf("want 'result: PASS', got %q", out)
		}
		if !strings.Contains(out, "lineage") {
			t.Errorf("seam preflight should run the lineage check, got %q", out)
		}
	})

	// Genuine drift: corrupt a managed file and fix the origin to match the
	// corrupted content so the divergence is genuine drift (stale-but-unedited:
	// origin==live, live!=staged) — NOT a consumer edit (non-failing) and NOT
	// F6 migration-stalled (which fires when hadOrigin is false). This keeps
	// the test's "preflight FAILs on real drift" intent intact.
	corruptManaged(t, root, seamManagedProbe)
	fixOriginToLive(t, root, seamManagedProbe)
	runWithCwd(t, root, func() {
		cmd, buf := newOutCmd()
		if err := runPreflight(cmd, nil); err == nil {
			t.Fatalf("seam preflight drift: want non-nil, got nil (out=%q)", buf.String())
		}
		if out := buf.String(); !strings.Contains(out, "managed-drift") || !strings.Contains(out, "FAIL") {
			t.Errorf("want managed-drift FAIL, got %q", out)
		}
	})
}

// TestOriginHashStore_RegularFile_SurvivesClone is the F7 clone-safety lock.
// origin-hashes.json is COMMITTED binary-owned platform state (not gitignored):
// losing it on clone loses the protection baseline. A git clone copies regular
// tracked files — so the sidecar MUST be a regular file (not a symlink, pipe, or
// special device) with valid JSON containing managed-path entries. This is the
// property that makes the committed-sidecar convention safe: clone reproduces
// the exact regular file, preserving the origin baseline across clone lifecycle.
// (Uninstall-removal is covered by TestSeamUninstall_RemovesOriginHashStoreSoReinstallReseeds;
// this test covers the clone-retention half of the lifecycle.)
func TestOriginHashStore_RegularFile_SurvivesClone(t *testing.T) {
	root := t.TempDir()
	seamInstallInto(t, root)

	sidecar := originhash.FilePath(root)

	// 1. Must exist as a REGULAR file (the property git clone preserves).
	info, err := os.Stat(sidecar)
	if err != nil {
		t.Fatalf("origin-hashes.json should exist after install: %v", err)
	}
	if !info.Mode().IsRegular() {
		t.Fatalf("origin-hashes.json must be a regular file (survives git clone); got mode %v", info.Mode())
	}

	// 2. Must be valid JSON with the expected schema (proves it is the real
	// protection baseline, not a corrupt/empty placeholder). Read+parse here
	// independently of originhash.Read to assert the on-disk bytes are valid.
	raw, err := os.ReadFile(sidecar)
	if err != nil {
		t.Fatalf("read origin-hashes.json: %v", err)
	}
	var store originhash.Store
	if err := json.Unmarshal(raw, &store); err != nil {
		t.Fatalf("origin-hashes.json must be valid JSON: %v", err)
	}
	if store.SchemaVersion == "" {
		t.Errorf("origin-hashes.json schema_version must be non-empty")
	}
	// 3. Must contain managed-path entries (the baseline is non-empty after a
	// real install — every platform_managed file gets an origin hash).
	if len(store.OriginHashes) == 0 {
		t.Errorf("origin-hashes.json must contain managed-path origin hashes after install; got empty map")
	}

	// 4. Cross-check: originhash.Read agrees the on-disk store is valid + non-nil
	// (the strict reader Apply uses). This binds the on-disk JSON validity to the
	// contract the apply path enforces.
	parsed, err := originhash.Read(root)
	if err != nil {
		t.Errorf("originhash.Read should succeed on the on-disk store: %v", err)
	}
	if parsed == nil {
		t.Errorf("originhash.Read should return non-nil store after install")
	}
}
