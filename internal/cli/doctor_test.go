package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeProfileOverlays seeds a target dir with a schema-valid
// .vh-agent-harness/vh-harness-profile.yml whose overlays list is names. This
// is what activeOverlays(target) reads (it validates via HarnessProfile, so the
// fixture must be conformant — a bare `overlays: [name]` is). An empty names
// slice writes a profile with no overlays key (core-only).
func writeProfileOverlays(t *testing.T, target string, names ...string) {
	t.Helper()
	dir := filepath.Join(target, ".vh-agent-harness")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	body := ""
	if len(names) > 0 {
		body = "overlays:\n"
		for _, n := range names {
			body += "  - " + n + "\n"
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "vh-harness-profile.yml"), []byte(body), 0o644); err != nil {
		t.Fatalf("write profile: %v", err)
	}
}

// writePermissionPack writes a permission-pack.jsonc body under
// .vh-agent-harness/overlays/<pack>/ for the named pack. body may include
// JSONC comments/trailing commas — the detection logic strips them, so tests can
// exercise realistic pack shapes.
func writePermissionPack(t *testing.T, target, pack, body string) {
	t.Helper()
	dir := filepath.Join(target, ".vh-agent-harness", "overlays", pack)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "permission-pack.jsonc"), []byte(body), 0o644); err != nil {
		t.Fatalf("write permission-pack.jsonc: %v", err)
	}
}

// writeOpencodeJSONC writes the repo-root opencode.jsonc fixture.
func writeOpencodeJSONC(t *testing.T, target, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(target, "opencode.jsonc"), []byte(body), 0o644); err != nil {
		t.Fatalf("write opencode.jsonc: %v", err)
	}
}

// TestOverlayPermissionState_FailWhenPlaceholder: an active overlay carrying a
// permission-pack.jsonc plus an opencode.jsonc that contains a `__placeholder__`
// sentinel (Signal A) must FAIL and name the resolver script so an operator knows
// the exact recovery command. NOTE: the harness scaffolder (`overlay new`) never
// emits this sentinel (it writes resolved values); this exercises the defensive
// branch that catches hand-authored packs using the sentinel. Signal B (edge
// check) is the primary detector — see TestOverlayPermissionState_FailWhenEdgeMissing.
func TestOverlayPermissionState_FailWhenPlaceholder(t *testing.T) {
	dir := t.TempDir()
	writeProfileOverlays(t, dir, "myoverlay")
	writePermissionPack(t, dir, "myoverlay", `{
  "agents": {
    "myoverlay-agent": {
      "permission": { "bash": ["__placeholder__"], "task": ["__placeholder__"] }
    }
  }
}`)
	writeOpencodeJSONC(t, dir, `{
  // opencode.jsonc with an unresolved overlay placeholder.
  "agents": {
    "build": { "permission": { "bash": ["__placeholder__"] } }
  }
}`)

	r := checkOverlayPermissionState(dir)
	if r.tier != tierFail {
		t.Fatalf("want FAIL when __placeholder__ present, got %s: %s", r.tier, r.detail)
	}
	if !strings.Contains(r.detail, overlayPermRecoveryCmd) {
		t.Errorf("FAIL detail should name the recovery command %q; got %q", overlayPermRecoveryCmd, r.detail)
	}
}

// TestOverlayPermissionState_FailWhenEdgeMissing: Signal B — the placeholder
// has been removed (Signal A clean) but a declared pack agent's delegateFrom
// task edge (`"<agent>": "allow"|"ask"`) is absent from opencode.jsonc. Must
// FAIL and name the resolver.
func TestOverlayPermissionState_FailWhenEdgeMissing(t *testing.T) {
	dir := t.TempDir()
	writeProfileOverlays(t, dir, "myoverlay")
	writePermissionPack(t, dir, "myoverlay", `{
  "agents": {
    "myoverlay-agent": { "permission": {} }
  }
}`)
	// No __placeholder__ anywhere, and no "myoverlay-agent": "allow" edge.
	writeOpencodeJSONC(t, dir, `{
  "agents": {
    "build": { "permission": { "bash": ["ls"] } }
  }
}`)

	r := checkOverlayPermissionState(dir)
	if r.tier != tierFail {
		t.Fatalf("want FAIL when delegateFrom edge missing, got %s: %s", r.tier, r.detail)
	}
	if !strings.Contains(r.detail, overlayPermRecoveryCmd) {
		t.Errorf("FAIL detail should name the recovery command; got %q", r.detail)
	}
	if !strings.Contains(r.detail, "myoverlay-agent") {
		t.Errorf("FAIL detail should name the missing-edge agent; got %q", r.detail)
	}
}

// TestOverlayPermissionState_CleanWhenResolved: an overlay pack whose agents all
// have their resolved `"<agent>": "allow"` edges present in opencode.jsonc, with
// no __placeholder__ anywhere, must PASS (resolver has been run).
func TestOverlayPermissionState_CleanWhenResolved(t *testing.T) {
	dir := t.TempDir()
	writeProfileOverlays(t, dir, "myoverlay")
	writePermissionPack(t, dir, "myoverlay", `{
  "agents": {
    "myoverlay-agent": { "permission": {} }
  }
}`)
	// The resolver injects delegateFrom edges as task entries into orchestrators;
	// here "myoverlay-agent": "allow" appears under the coordination orchestrator.
	writeOpencodeJSONC(t, dir, `{
  "agents": {
    "coordination": {
      "delegateFrom": {
        "myoverlay-agent": "allow"
      }
    }
  }
}`)

	r := checkOverlayPermissionState(dir)
	if r.tier != tierPass {
		t.Fatalf("want PASS when overlay agents resolved, got %s: %s", r.tier, r.detail)
	}
}

// TestOverlayPermissionState_SilentCoreOnly: a repo with no active overlays
// (no vh-harness-profile.yml at all) is core-only — the resolver is not
// required. The check must stay SILENT (PASS) so doctor stays HEALTHY.
func TestOverlayPermissionState_SilentCoreOnly(t *testing.T) {
	dir := t.TempDir()
	// No profile written -> activeOverlays returns nil -> core-only.

	r := checkOverlayPermissionState(dir)
	if r.tier != tierPass {
		t.Fatalf("want PASS for core-only (no overlays), got %s: %s", r.tier, r.detail)
	}
}

// TestOverlayPermissionState_SilentWhenNoPacks: a repo with active overlays but
// none of them ship a permission-pack.jsonc — the resolver has nothing to
// resolve. Must stay SILENT (PASS).
func TestOverlayPermissionState_SilentWhenNoPacks(t *testing.T) {
	dir := t.TempDir()
	writeProfileOverlays(t, dir, "packless")
	// overlay dir exists but NO permission-pack.jsonc inside it.
	if err := os.MkdirAll(filepath.Join(dir, ".vh-agent-harness", "overlays", "packless"), 0o755); err != nil {
		t.Fatalf("mkdir overlay: %v", err)
	}
	writeOpencodeJSONC(t, dir, `{}`)

	r := checkOverlayPermissionState(dir)
	if r.tier != tierPass {
		t.Fatalf("want PASS when no overlay carries a permission-pack, got %s: %s", r.tier, r.detail)
	}
}

// TestOverlayPermissionState_SkipWhenNoOpencodeJSONC: overlay + pack present but
// opencode.jsonc absent — the managed-drift check already FAILs the missing
// managed file, so overlay-perm SKIPS rather than double-reporting existence.
func TestOverlayPermissionState_SkipWhenNoOpencodeJSONC(t *testing.T) {
	dir := t.TempDir()
	writeProfileOverlays(t, dir, "myoverlay")
	writePermissionPack(t, dir, "myoverlay", `{"agents":{"x":{}}}`)
	// Deliberately no opencode.jsonc.

	r := checkOverlayPermissionState(dir)
	if r.tier != tierSkip {
		t.Fatalf("want SKIP when opencode.jsonc absent (managed-drift owns that), got %s: %s", r.tier, r.detail)
	}
}
