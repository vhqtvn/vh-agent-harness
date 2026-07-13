package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// TestMain isolates the whole cli-package test binary from the operator's real
// user-level config. Several checks resolve user-level files via
// os.UserConfigDir() (which follows $XDG_CONFIG_HOME on Unix); a test must
// never depend on the operator's real home config, so point XDG at a throwaway
// temp dir for every test in the package. Individual tests that need their own
// user-level fixtures still override this via isolateXDG (t.Setenv restores
// afterward). This keeps the TestSeam* doctor-asserts-HEALTHY tests stable
// even when the operator has a (possibly corrupt) user-level config on disk.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "vh-cli-test-xdg-")
	if err != nil {
		fmt.Fprintln(os.Stderr, "TestMain: mkdir temp:", err)
		os.Exit(1)
	}
	defer os.RemoveAll(dir)
	os.Setenv("XDG_CONFIG_HOME", dir)
	os.Exit(m.Run())
}

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

// --- auto-classifier config shape check (checkAutoGateConfig) ---
//
// These cases pin the SCHEMA ENVELOPE of the auto-classifier-pilot overlay's
// config files. The SCHEMA SOURCE OF TRUTH is the JS plugin
// templates/overlays/auto-classifier-pilot/plugins/auto-tool-gate.js
// (DEFAULT_PLUGIN_CONFIG ~L376-385, DEFAULT_LLM_CONFIG ~L403-413,
// normalizePluginConfig ~L480-521, normalizeLlmConfig ~L543-588). If the JS
// schema gains/drops/retypes a field or widens an enum, update Go's validators
// (validateAutoGatePluginConfig / validateAutoGateLlmConfig) AND these pinning
// tests together — the drift contract on checkAutoGateConfig cross-references the
// JS line ranges.

// isolateXDG points os.UserConfigDir() at an isolated temp dir for the test so
// user-level auto-gate config resolution does not leak real-environment files
// into the check. User-level files live under <root>/vh-agent-harness/. Returns
// the XDG root so a test that wants a user-level file can write under it.
func isolateXDG(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", root)
	return root
}

// writeAutoGateConfig writes an auto-gate config file under the project-level
// repo-configs/ dir (the rendered location). kind selects plugin
// (auto-gate-config.json) or llm (auto-gate-llm.json). body is written verbatim so
// tests can exercise corrupt JSON shapes.
func writeAutoGateConfig(t *testing.T, target, kind, body string) {
	t.Helper()
	dir := filepath.Join(target, ".opencode", "repo-configs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	name := "auto-gate-config.json"
	if kind == "llm" {
		name = "auto-gate-llm.json"
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

// writeUserAutoGateConfig writes an auto-gate config file under a user-level XDG
// root (isolateXDG's return value). kind selects plugin/llm.
func writeUserAutoGateConfig(t *testing.T, xdgRoot, kind, body string) {
	t.Helper()
	dir := filepath.Join(xdgRoot, "vh-agent-harness")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	name := "auto-gate-config.json"
	if kind == "llm" {
		name = "auto-gate-llm.json"
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

// TestAutoGateConfig_SkipWhenUnselectedAndNoFiles: overlay unselected + no config
// files present → clean no-op (tierSkip). This is the common core-only case.
func TestAutoGateConfig_SkipWhenUnselectedAndNoFiles(t *testing.T) {
	dir := t.TempDir()
	isolateXDG(t) // no user-level files leak in

	r := checkAutoGateConfig(dir)
	if r.tier != tierSkip {
		t.Fatalf("want SKIP when unselected + no files, got %s: %s", r.tier, r.detail)
	}
}

// TestAutoGateConfig_PassWhenSelectedAndValid: overlay selected + a valid plugin
// config + a valid LLM config → PASS (all present files shape-valid).
func TestAutoGateConfig_PassWhenSelectedAndValid(t *testing.T) {
	dir := t.TempDir()
	isolateXDG(t)
	writeProfileOverlays(t, dir, "auto-classifier-pilot")
	writeAutoGateConfig(t, dir, "plugin", `{
	  "enabled": true, "mode": "enforce", "stubVerdict": "allow",
	  "promptFile": "", "replyMode": "once", "onUncertain": "reject",
	  "harnessContext": true, "guides": true
	}`)
	writeAutoGateConfig(t, dir, "llm", `{
	  "modelEndpoint": "https://x", "modelEndpointEnv": "EP", "model": "m",
	  "apiKey": "", "apiKeyEnv": "KEY", "timeoutMs": 5000,
	  "maxRetries": 2, "retryDelayMs": 100, "leaves": []
	}`)

	r := checkAutoGateConfig(dir)
	if r.tier != tierPass {
		t.Fatalf("want PASS when selected + valid configs, got %s: %s", r.tier, r.detail)
	}
}

// TestAutoGateConfig_FailCorruptJson: a present plugin config with corrupt JSON
// (trailing comma) → FAIL (present-but-invalid breaks doctor).
func TestAutoGateConfig_FailCorruptJson(t *testing.T) {
	dir := t.TempDir()
	isolateXDG(t)
	writeProfileOverlays(t, dir, "auto-classifier-pilot")
	writeAutoGateConfig(t, dir, "plugin", `{"mode": "enforce",}`) // trailing comma

	r := checkAutoGateConfig(dir)
	if r.tier != tierFail {
		t.Fatalf("want FAIL for corrupt JSON, got %s: %s", r.tier, r.detail)
	}
	if !strings.Contains(r.detail, "invalid JSON") {
		t.Errorf("FAIL detail should mention invalid JSON; got %q", r.detail)
	}
}

// TestAutoGateConfig_FailBadEnum: a plugin config with a bad enum value
// (mode: "audited") → FAIL naming the field.
func TestAutoGateConfig_FailBadEnum(t *testing.T) {
	dir := t.TempDir()
	isolateXDG(t)
	writeProfileOverlays(t, dir, "auto-classifier-pilot")
	writeAutoGateConfig(t, dir, "plugin", `{"mode":"audited"}`)

	r := checkAutoGateConfig(dir)
	if r.tier != tierFail {
		t.Fatalf("want FAIL for bad enum, got %s: %s", r.tier, r.detail)
	}
	if !strings.Contains(r.detail, "mode") {
		t.Errorf("FAIL detail should name the bad field; got %q", r.detail)
	}
}

// TestAutoGateConfig_FailWrongType: a plugin config with a wrong type
// (enabled: "yes" — string instead of bool) → FAIL.
func TestAutoGateConfig_FailWrongType(t *testing.T) {
	dir := t.TempDir()
	isolateXDG(t)
	writeProfileOverlays(t, dir, "auto-classifier-pilot")
	writeAutoGateConfig(t, dir, "plugin", `{"enabled":"yes"}`)

	r := checkAutoGateConfig(dir)
	if r.tier != tierFail {
		t.Fatalf("want FAIL for wrong type, got %s: %s", r.tier, r.detail)
	}
	if !strings.Contains(r.detail, "enabled") {
		t.Errorf("FAIL detail should name the mistyped field; got %q", r.detail)
	}
}

// TestAutoGateConfig_WarnUnknownField: an unknown top-level field → WARN (not
// FAIL). Known fields with valid values stay clean; only the stray key warns.
func TestAutoGateConfig_WarnUnknownField(t *testing.T) {
	dir := t.TempDir()
	isolateXDG(t)
	writeProfileOverlays(t, dir, "auto-classifier-pilot")
	writeAutoGateConfig(t, dir, "plugin", `{"mode":"audit","bogusField":1}`)

	r := checkAutoGateConfig(dir)
	if r.tier != tierWarn {
		t.Fatalf("want WARN for unknown field, got %s: %s", r.tier, r.detail)
	}
	if !strings.Contains(r.detail, "bogusField") {
		t.Errorf("WARN detail should name the unknown field; got %q", r.detail)
	}
}

// TestAutoGateConfig_FailLeavesNotArray: an LLM config where `leaves` is not an
// array → FAIL (wrong type).
func TestAutoGateConfig_FailLeavesNotArray(t *testing.T) {
	dir := t.TempDir()
	isolateXDG(t)
	writeProfileOverlays(t, dir, "auto-classifier-pilot")
	writeAutoGateConfig(t, dir, "llm", `{"leaves":"notarray"}`)

	r := checkAutoGateConfig(dir)
	if r.tier != tierFail {
		t.Fatalf("want FAIL for leaves not array, got %s: %s", r.tier, r.detail)
	}
}

// TestAutoGateConfig_WarnLeavesNonObjectElement: an LLM config where `leaves` is
// an array but contains a non-object element → WARN (shallow check, no FAIL).
func TestAutoGateConfig_WarnLeavesNonObjectElement(t *testing.T) {
	dir := t.TempDir()
	isolateXDG(t)
	writeProfileOverlays(t, dir, "auto-classifier-pilot")
	writeAutoGateConfig(t, dir, "llm", `{"leaves":["ok", 42, {"model":"x"}]}`)

	r := checkAutoGateConfig(dir)
	if r.tier != tierWarn {
		t.Fatalf("want WARN for non-object leaf element, got %s: %s", r.tier, r.detail)
	}
	if !strings.Contains(r.detail, "leaves") {
		t.Errorf("WARN detail should name the leaves field; got %q", r.detail)
	}
}

// TestAutoGateConfig_FailWhenUnselectedButCorruptFilePresent: the safety net —
// overlay NOT selected but a corrupt config file exists on disk → FAIL (a stale
// file does not silently break the config that a selected worktree depends on).
func TestAutoGateConfig_FailWhenUnselectedButCorruptFilePresent(t *testing.T) {
	dir := t.TempDir()
	isolateXDG(t)
	// No profile written -> overlay unselected. But a corrupt plugin config exists.
	writeAutoGateConfig(t, dir, "plugin", `{"mode": oops}`) // unquoted value

	r := checkAutoGateConfig(dir)
	if r.tier != tierFail {
		t.Fatalf("want FAIL (safety net) when corrupt file present even if overlay unselected, got %s: %s", r.tier, r.detail)
	}
}

// TestAutoGateConfig_FailNegativeMaxRetries: an LLM config with a negative
// maxRetries → FAIL (must be a non-negative integer). Also covers retryDelayMs.
func TestAutoGateConfig_FailNegativeMaxRetries(t *testing.T) {
	dir := t.TempDir()
	isolateXDG(t)
	writeProfileOverlays(t, dir, "auto-classifier-pilot")
	writeAutoGateConfig(t, dir, "llm", `{"maxRetries":-1}`)

	r := checkAutoGateConfig(dir)
	if r.tier != tierFail {
		t.Fatalf("want FAIL for negative maxRetries, got %s: %s", r.tier, r.detail)
	}
	if !strings.Contains(r.detail, "maxRetries") {
		t.Errorf("FAIL detail should name maxRetries; got %q", r.detail)
	}
}

// TestAutoGateConfig_FailFractionalMaxRetries: a fractional value for an
// integer-constrained field (maxRetries: 2.5) → FAIL; 2.0 is accepted.
func TestAutoGateConfig_FailFractionalMaxRetries(t *testing.T) {
	dir := t.TempDir()
	isolateXDG(t)
	writeProfileOverlays(t, dir, "auto-classifier-pilot")
	writeAutoGateConfig(t, dir, "llm", `{"maxRetries":2.5}`)

	r := checkAutoGateConfig(dir)
	if r.tier != tierFail {
		t.Fatalf("want FAIL for fractional maxRetries, got %s: %s", r.tier, r.detail)
	}
}

// TestAutoGateConfig_AcceptsZeroFractionFloat: a float64 with a zero fraction
// part (maxRetries: 2.0) is accepted as an integer-constrained value. This pins
// the "2.0 ok" rule from the validation spec.
func TestAutoGateConfig_AcceptsZeroFractionFloat(t *testing.T) {
	dir := t.TempDir()
	isolateXDG(t)
	writeProfileOverlays(t, dir, "auto-classifier-pilot")
	writeAutoGateConfig(t, dir, "llm", `{"maxRetries":2.0,"retryDelayMs":0}`)

	r := checkAutoGateConfig(dir)
	if r.tier != tierPass {
		t.Fatalf("want PASS for zero-fraction floats, got %s: %s", r.tier, r.detail)
	}
}

// TestAutoGateConfig_PluginKnownFieldsPinned pins the plugin-config known field
// set against the SCHEMA SOURCE OF TRUTH (DEFAULT_PLUGIN_CONFIG ~L376-385,
// normalizePluginConfig ~L480-521 in auto-tool-gate.js). Every known field with a
// valid value — including the enum edges (live-tiered, fail, always, passthrough)
// — must yield PASS. If the JS schema gains/drops a field, update Go's known set
// AND this test together.
func TestAutoGateConfig_PluginKnownFieldsPinned(t *testing.T) {
	dir := t.TempDir()
	isolateXDG(t)
	writeProfileOverlays(t, dir, "auto-classifier-pilot")
	writeAutoGateConfig(t, dir, "plugin", `{
	  "enabled": true, "mode": "live-tiered", "stubVerdict": "fail",
	  "promptFile": "/x/p.md", "replyMode": "always", "onUncertain": "passthrough",
	  "harnessContext": false, "guides": false
	}`)

	r := checkAutoGateConfig(dir)
	if r.tier != tierPass {
		t.Fatalf("all known plugin fields valid (incl enum edges) should PASS; got %s: %s", r.tier, r.detail)
	}
}

// TestAutoGateConfig_LlmKnownFieldsPinned pins the LLM-config known field set
// against the SCHEMA SOURCE OF TRUTH (DEFAULT_LLM_CONFIG ~L403-413,
// normalizeLlmConfig ~L543-588 in auto-tool-gate.js). Every known field with a
// valid value (incl leaves with object elements and 0-valued integers) must PASS.
func TestAutoGateConfig_LlmKnownFieldsPinned(t *testing.T) {
	dir := t.TempDir()
	isolateXDG(t)
	writeProfileOverlays(t, dir, "auto-classifier-pilot")
	writeAutoGateConfig(t, dir, "llm", `{
	  "modelEndpoint": "https://x", "modelEndpointEnv": "EP", "model": "m",
	  "apiKey": "k", "apiKeyEnv": "KEY", "timeoutMs": 1,
	  "maxRetries": 0, "retryDelayMs": 0, "leaves": [{"model":"a"}, {"model":"b"}]
	}`)

	r := checkAutoGateConfig(dir)
	if r.tier != tierPass {
		t.Fatalf("all known llm fields valid should PASS; got %s: %s", r.tier, r.detail)
	}
}

// TestAutoGateConfig_SchemaParityWithJSSource is the SELF-ENFORCING drift
// contract between the Go doctor schema envelope and the live JS source of
// truth (templates/overlays/auto-classifier-pilot/plugins/auto-tool-gate.js).
// It parses DEFAULT_PLUGIN_CONFIG and DEFAULT_LLM_CONFIG out of the JS file and
// asserts their top-level field sets equal the Go known-field slices
// (autoGatePluginKnownFields / autoGateLlmKnownFields). If a future slice adds a
// field to the JS and forgets the Go envelope, this test fails with a diff
// naming the added/removed fields — instead of doctor silently WARN-ing on the
// new valid field (the gap the existing Plugin/LlmKnownFieldsPinned tests could
// not catch, since they pin a hardcoded set). The pinned tests are retained as
// belt-and-suspenders: they additionally prove a VALID config end-to-end PASSES
// through checkAutoGateConfig (this test only proves the field SET matches).
func TestAutoGateConfig_SchemaParityWithJSSource(t *testing.T) {
	// Reuse the package's existing findModuleRoot (walks up from the test cwd to
	// go.mod) — the cli test binary runs from internal/cli/, so the JS overlay
	// path is only repo-relative once the module root is resolved.
	repoRoot := findModuleRoot(t)
	jsPath := filepath.Join(repoRoot, "templates", "overlays",
		"auto-classifier-pilot", "plugins", "auto-tool-gate.js")
	js, err := os.ReadFile(jsPath)
	if err != nil {
		t.Fatalf("read JS source of truth %s: %v\n"+
			"(the parity test requires the auto-tool-gate.js overlay unit to exist "+
			"at its repo-relative path)", jsPath, err)
	}

	t.Run("plugin_config", func(t *testing.T) {
		jsFields, err := extractDefaultConfigFields(js, "DEFAULT_PLUGIN_CONFIG")
		if err != nil {
			t.Fatalf("extract DEFAULT_PLUGIN_CONFIG fields: %v", err)
		}
		assertSchemaParity(t, jsFields, autoGatePluginKnownFields, "plugin")
	})
	t.Run("llm_config", func(t *testing.T) {
		jsFields, err := extractDefaultConfigFields(js, "DEFAULT_LLM_CONFIG")
		if err != nil {
			t.Fatalf("extract DEFAULT_LLM_CONFIG fields: %v", err)
		}
		assertSchemaParity(t, jsFields, autoGateLlmKnownFields, "llm")
	})
}

// extractDefaultConfigFields parses the top-level field names out of a
// `const <blockName> = Object.freeze({ ... });` block in the JS source. It
// captures the identifier at the start of each indented `<ident>:` line inside
// the block and stops at the closing `});`. Nested fields (e.g. entries inside
// `leaves: []`) are intentionally ignored — only the top-level envelope is the
// schema contract. Inline trailing comments after the value are harmless (the
// regex matches only the leading identifier). Returns the fields in source
// order; callers sort before comparing.
func extractDefaultConfigFields(js []byte, blockName string) ([]string, error) {
	marker := regexp.MustCompile(regexp.QuoteMeta(blockName) + `\s*=\s*Object\.freeze\(\s*\{`)
	loc := marker.FindIndex(js)
	if loc == nil {
		return nil, fmt.Errorf("marker %q = Object.freeze({ not found in JS source", blockName)
	}
	body := string(js[loc[1]:])
	fieldRe := regexp.MustCompile(`^[ \t]+([A-Za-z_$][A-Za-z0-9_$]*):`)
	closeRe := regexp.MustCompile(`^[ \t]*\}\s*\)\s*;`)
	var fields []string
	for _, line := range strings.Split(body, "\n") {
		if closeRe.MatchString(line) {
			break
		}
		if m := fieldRe.FindStringSubmatch(line); m != nil {
			fields = append(fields, m[1])
		}
	}
	if len(fields) == 0 {
		return nil, fmt.Errorf("no top-level fields parsed from %q block (marker matched but body empty?)", blockName)
	}
	return fields, nil
}

// assertSchemaParity compares the JS-extracted field set against the Go known
// field slice and fails with a diff naming the JS-only and Go-only fields, so
// the fix (add the new JS field to the Go slice, or drop a stale Go field) is
// obvious from the failure message.
func assertSchemaParity(t *testing.T, jsFields, goFields []string, label string) {
	t.Helper()
	jsSet := make(map[string]bool, len(jsFields))
	for _, f := range jsFields {
		jsSet[f] = true
	}
	goSet := make(map[string]bool, len(goFields))
	for _, f := range goFields {
		goSet[f] = true
	}
	var jsOnly, goOnly []string
	for f := range jsSet {
		if !goSet[f] {
			jsOnly = append(jsOnly, f)
		}
	}
	for f := range goSet {
		if !jsSet[f] {
			goOnly = append(goOnly, f)
		}
	}
	sort.Strings(jsOnly)
	sort.Strings(goOnly)
	if len(jsOnly) > 0 || len(goOnly) > 0 {
		t.Errorf("%s schema drifted from JS source of truth "+
			"(auto-tool-gate.js DEFAULT_%s_CONFIG): "+
			"JS-only (add to Go autoGate...KnownFields) = %v; "+
			"Go-only (remove stale field or add to JS) = %v",
			label, strings.ToUpper(label[:1])+label[1:], jsOnly, goOnly)
	}
}

// TestAutoGateConfig_UserLevelViaXDG: a user-level (XDG) config is resolved via
// os.UserConfigDir() and validated standalone. A bad user-level LLM config FAILs
// and the detail labels the file as user-level. This is the cross-platform XDG
// parity case (Linux dev container).
func TestAutoGateConfig_UserLevelViaXDG(t *testing.T) {
	dir := t.TempDir()
	xdg := isolateXDG(t)
	writeProfileOverlays(t, dir, "auto-classifier-pilot")
	// user-level plugin valid; user-level LLM has a negative retryDelayMs → FAIL.
	writeUserAutoGateConfig(t, xdg, "plugin", `{"mode":"live"}`)
	writeUserAutoGateConfig(t, xdg, "llm", `{"retryDelayMs":-5}`)

	r := checkAutoGateConfig(dir)
	if r.tier != tierFail {
		t.Fatalf("want FAIL for bad user-level config, got %s: %s", r.tier, r.detail)
	}
	if !strings.Contains(r.detail, "user") {
		t.Errorf("FAIL detail should label the user-level file; got %q", r.detail)
	}
	if !strings.Contains(r.detail, "retryDelayMs") {
		t.Errorf("FAIL detail should name the bad field; got %q", r.detail)
	}
}

// TestAutoGateConfig_MissingOptionalIsNotFailure: overlay selected but NO config
// files present at all → PASS (not SKIP, because selected; and not FAIL, because
// absence is the documented fail-safe default for both plugin and LLM configs).
func TestAutoGateConfig_MissingOptionalIsNotFailure(t *testing.T) {
	dir := t.TempDir()
	isolateXDG(t)
	writeProfileOverlays(t, dir, "auto-classifier-pilot")
	// No plugin or LLM config files written.

	r := checkAutoGateConfig(dir)
	if r.tier != tierPass {
		t.Fatalf("want PASS when selected but all configs absent (defaults apply), got %s: %s", r.tier, r.detail)
	}
}
