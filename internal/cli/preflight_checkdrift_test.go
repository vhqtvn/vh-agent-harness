package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vhqtvn/vh-agent-harness/internal/manifest"
)

// sha256Hex returns the "sha256:<hex>" digest of content, matching the on-disk
// hash format drift.Compute records in a manifest. The cli package test cannot
// import the unexported drift.hashFile helper, so this mirrors it exactly over
// an in-memory string (used only to seed manifest hashes for a staged fixture).
func sha256Hex(content string) string {
	sum := sha256.Sum256([]byte(content))
	return "sha256:" + hex.EncodeToString(sum[:])
}

// TestPreflight_CheckDrift_NamesPathAndBothRemedies is the end-to-end coverage
// for the legacy-manifest preflight drift codepath (checkDrift in preflight.go).
// Unlike checkManagedDrift (doctor + the seam preflight path, covered by
// TestManagedDrift_Divergent_NamesPathAndBothRemedies), the legacy checkDrift
// path-collection loop that extracts drifted/missing paths from
// drift.Compute's report.Entries and routes them through the shared
// formatManagedDriftFail formatter had NO dedicated integration test — only the
// shared formatter (TestFormatManagedDriftFail) was covered in isolation.
//
// The test stages a legacy manifest install (NOT a seam install — checkDrift is
// the non-seam codepath invoked from runPreflight's else branch), induces
// drifted + missing entries, invokes checkDrift directly, and asserts:
//   - it FAILs (drift blocks preflight),
//   - each drifted AND missing path is NAMED in the FAIL detail (self-routing —
//     the load-bearing surface-at-friction fix), and
//   - BOTH recovery remedies are present: the destructive `update` AND the
//     non-destructive overlay-pack-source promotion.
//
// Pins researches/decisions/2026-08-04-capability-discovery-audit.md §6 entry 1
// (the SIGNED OFF entry) for the legacy preflight path, parallel to the doctor/
// seam coverage already locked by TestManagedDrift_Divergent_NamesPathAndBothRemedies.
func TestPreflight_CheckDrift_NamesPathAndBothRemedies(t *testing.T) {
	root := t.TempDir()

	// On-disk files under .opencode/. drifted.md and ok.md are written with
	// their "original" bytes so the manifest can record a matching hash; drifted
	// is then diverged on disk below. missing.md is tracked by the manifest but
	// never written to disk (induces the Missing category).
	driftedRel := ".opencode/agents/drifted.md"
	missingRel := ".opencode/agents/missing.md"
	okRel := ".opencode/agents/ok.md"
	for _, rel := range []string{driftedRel, okRel} {
		abs := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", rel, err)
		}
		if err := os.WriteFile(abs, []byte("original"), 0o644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}

	// Build the legacy manifest tracking all three paths with ClassManaged.
	// ok + drifted hash their "original" bytes (ok stays in sync; drifted is
	// diverged next). missing records a real hash but the file is absent.
	m := manifest.New()
	m.Files[okRel] = manifest.File{Hash: sha256Hex("original"), Class: manifest.ClassManaged}
	m.Files[driftedRel] = manifest.File{Hash: sha256Hex("original"), Class: manifest.ClassManaged}
	m.Files[missingRel] = manifest.File{Hash: sha256Hex("never-on-disk-original"), Class: manifest.ClassManaged}

	// Diverge drifted.md so its disk hash no longer matches the manifest hash.
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(driftedRel)),
		[]byte("EDITED-DIVERGENT-BYTES"), 0o644); err != nil {
		t.Fatalf("diverge %s: %v", driftedRel, err)
	}
	// missing.md stays absent on disk (induces the Missing category).

	// checkDrift is the legacy-manifest preflight codepath; it reads only dir +
	// m from the loaded manifest, so a minimal in-memory struct exercises it
	// without the rest of the preflight table (node/eval.js/runtime).
	lm := &loadedManifest{dir: root, m: m}
	r := checkDrift(lm)

	if r.tier != tierFail {
		t.Fatalf("want FAIL for drifted+missing legacy manifest, got %s: %s", r.tier, r.detail)
	}
	// Names the drifted path (self-routing — the load-bearing fix).
	if !strings.Contains(r.detail, driftedRel) {
		t.Errorf("FAIL detail should name the drifted path %q; got %q", driftedRel, r.detail)
	}
	// Names the missing path.
	if !strings.Contains(r.detail, missingRel) {
		t.Errorf("FAIL detail should name the missing path %q; got %q", missingRel, r.detail)
	}
	// The drifted/missing path-segment labels are present. Use the colon form
	// ("drifted:"/"missing:") so the count header ("1 drifted,") is not a match.
	if !strings.Contains(r.detail, "drifted:") {
		t.Errorf("FAIL detail should carry a drifted path segment; got %q", r.detail)
	}
	if !strings.Contains(r.detail, "missing:") {
		t.Errorf("FAIL detail should carry a missing path segment; got %q", r.detail)
	}
	// Destructive remedy.
	if !strings.Contains(r.detail, "vh-agent-harness update") || !strings.Contains(r.detail, "DESTRUCTIVE") {
		t.Errorf("FAIL detail should name the destructive `update` remedy; got %q", r.detail)
	}
	// Non-destructive overlay-promotion remedy.
	if !strings.Contains(r.detail, "overlay pack source") || !strings.Contains(r.detail, ".vh-agent-harness/overlays/<pack>/") {
		t.Errorf("FAIL detail should name the non-destructive overlay-pack remedy; got %q", r.detail)
	}
}

// TestPreflight_CheckDrift_OnlyUnexpected_StillAppendsRemedies pins advisory A4:
// when a checkDrift report has ONLY drift.Unexpected entries (no drifted/missing
// paths), formatManagedDriftFail still appends BOTH remedies via the shared
// driftRecoverySuffix. This is message noise only — the remedies reference
// drifted/missing recovery but no such paths triggered the FAIL — and is
// acceptable because the recovery commands are identical regardless of which
// category triggered. The test documents the current behavior so a future
// narrowing (e.g. gating the suffix on drifted+missing presence) is a
// deliberate, reviewed change rather than a silent drift.
func TestPreflight_CheckDrift_OnlyUnexpected_StillAppendsRemedies(t *testing.T) {
	root := t.TempDir()

	// A stray file under .opencode/ not tracked by the manifest is the SOLE
	// problem: Unexpected only, no drifted/missing entries.
	strayRel := ".opencode/agents/stray-extra.md"
	abs := filepath.Join(root, filepath.FromSlash(strayRel))
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", strayRel, err)
	}
	if err := os.WriteFile(abs, []byte("not tracked by the manifest"), 0o644); err != nil {
		t.Fatalf("write %s: %v", strayRel, err)
	}

	// Empty manifest (no tracked paths) so the unexpected walk is the only
	// signal — drift.Compute classifies no manifest path and flags the stray.
	m := manifest.New()

	lm := &loadedManifest{dir: root, m: m}
	r := checkDrift(lm)

	if r.tier != tierFail {
		t.Fatalf("want FAIL for unexpected-only report, got %s: %s", r.tier, r.detail)
	}
	// The summary header counts the unexpected entry.
	if !strings.Contains(r.detail, "1 unexpected") {
		t.Errorf("FAIL detail should count the unexpected entry; got %q", r.detail)
	}
	// No drifted/missing paths exist, so no path segments must be named (no
	// false path routing). The colon form distinguishes the segment label from
	// the count header ("0 drifted," carries no colon after drifted).
	if strings.Contains(r.detail, "drifted:") {
		t.Errorf("FAIL detail should not carry a drifted path segment (none exist); got %q", r.detail)
	}
	if strings.Contains(r.detail, "missing:") {
		t.Errorf("FAIL detail should not carry a missing path segment (none exist); got %q", r.detail)
	}
	// Advisory A4: remedies still appended (the documented message noise) — pins
	// current behavior so a future change is deliberate.
	if !strings.Contains(r.detail, "vh-agent-harness update") || !strings.Contains(r.detail, "DESTRUCTIVE") {
		t.Errorf("FAIL detail should still name the destructive remedy (advisory A4 noise); got %q", r.detail)
	}
	if !strings.Contains(r.detail, "overlay pack source") {
		t.Errorf("FAIL detail should still name the non-destructive remedy (advisory A4 noise); got %q", r.detail)
	}
}
