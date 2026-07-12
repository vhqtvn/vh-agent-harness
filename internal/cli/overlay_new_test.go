package cli

// overlay_new_test.go covers `vh-agent-harness overlay new` — the Slice 2
// scaffolder. It is the riskiest slice (it mutates the platform_armed
// vh-harness-profile.yml), so coverage targets the profile-append safety, the
// strict dry-run, the name-collision / existing-file rejection, and the unit-
// flag combinations.
//
// The schema-level append (schema.HarnessProfile{}.AppendOverlay) is the
// single safe mutation site; the command wires it. Both layers are tested.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vhqtvn/vh-agent-harness/internal/schema"
)

// freshOverlayTarget returns a temp dir with a .vh-agent-harness/ dir present
// (the minimum `overlay new` requires). Optionally seeds a profile body.
func freshOverlayTarget(t *testing.T, profileBody string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".vh-agent-harness"), 0o755); err != nil {
		t.Fatalf("mkdir .vh-agent-harness: %v", err)
	}
	if profileBody != "" {
		writeProfile(t, dir, profileBody)
	}
	return dir
}

// runOverlayNewIn runs `overlay new <args[0]>` against target with the given
// unit flags + dryRun, returning the combined output buffer + the returned
// error. It resets the package-level overlayNewFl so tests do not leak flag
// state. target is set as an absolute path so cwd never matters.
func runOverlayNewIn(t *testing.T, target, agent, command, skill string, dryRun bool, args ...string) (string, error) {
	t.Helper()
	overlayNewFl = &overlayNewFlags{
		target:  target,
		agent:   agent,
		command: command,
		skill:   skill,
		dryRun:  dryRun,
	}
	cmd, buf := newOutCmd()
	err := runOverlayNew(cmd, args)
	return buf.String(), err
}

// readProfileOverlays reads the overlays slice back from the on-disk profile at
// target. Fails the test if the profile is absent.
func readProfileOverlays(t *testing.T, target string) []string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(target, harnessProfileName))
	if err != nil {
		t.Fatalf("read profile: %v", err)
	}
	return extractOverlays(raw)
}

// packExists reports whether the pack dir is present on disk.
func packExists(target, name string) bool {
	return isExistingDir(filepath.Join(target, ".vh-agent-harness", "overlays", name))
}

// packFileExists reports whether a file inside the pack dir is present.
func packFileExists(target, name, rel string) bool {
	_, err := os.Stat(filepath.Join(target, ".vh-agent-harness", "overlays", name, filepath.FromSlash(rel)))
	return err == nil
}

// --- schema.HarnessProfile{}.AppendOverlay: the safe mutation site ------------

// TestAppendOverlay_EmptyRawBuildsFreshInstance confirms an absent/empty profile
// yields a fresh conformant instance carrying only `overlays: [name]`, with
// added=true.
func TestAppendOverlay_EmptyRawBuildsFreshInstance(t *testing.T) {
	merged, added, err := (schema.HarnessProfile{}).AppendOverlay(nil, "alpha")
	if err != nil {
		t.Fatalf("AppendOverlay: %v", err)
	}
	if !added {
		t.Errorf("empty raw: added=false, want true")
	}
	if got := extractOverlays(merged); len(got) != 1 || got[0] != "alpha" {
		t.Errorf("empty raw: overlays=%v, want [alpha]", got)
	}
}

// TestAppendOverlay_ExistingEntriesAppended confirms a profile with existing
// overlays gets the new name appended (and the result is sorted/deduped).
func TestAppendOverlay_ExistingEntriesAppended(t *testing.T) {
	raw := []byte("profile: minimal\nmodules: [core]\noverlays: [beta]\npolicy_packs: []\n")
	merged, added, err := (schema.HarnessProfile{}).AppendOverlay(raw, "alpha")
	if err != nil {
		t.Fatalf("AppendOverlay: %v", err)
	}
	if !added {
		t.Errorf("existing entries: added=false, want true")
	}
	got := extractOverlays(merged)
	// marshalHarnessProfile sorts+dedupes, so order is alphabetical.
	want := []string{"alpha", "beta"}
	if len(got) != 2 || got[0] != "alpha" || got[1] != "beta" {
		t.Errorf("existing entries: overlays=%v, want %v", got, want)
	}
}

// TestAppendOverlay_AlreadyPresentNoDup confirms a name already in overlays is
// not duplicated, with added=false (caller may skip the write).
func TestAppendOverlay_AlreadyPresentNoDup(t *testing.T) {
	raw := []byte("profile: minimal\nmodules: [core]\noverlays: [alpha, beta]\npolicy_packs: []\n")
	merged, added, err := (schema.HarnessProfile{}).AppendOverlay(raw, "alpha")
	if err != nil {
		t.Fatalf("AppendOverlay: %v", err)
	}
	if added {
		t.Errorf("already present: added=true, want false")
	}
	got := extractOverlays(merged)
	if len(got) != 2 {
		t.Errorf("already present: overlays=%v, want exactly [alpha beta]", got)
	}
}

// TestAppendOverlay_RejectsInvalidProfile confirms the append refuses to mutate
// a schema-invalid profile (never silently re-serializes a malformed file).
func TestAppendOverlay_RejectsInvalidProfile(t *testing.T) {
	raw := []byte("profile: not_a_real_enum\nmodules: [core]\noverlays: [alpha]\n")
	_, _, err := (schema.HarnessProfile{}).AppendOverlay(raw, "beta")
	if err == nil {
		t.Errorf("invalid profile: expected error, got nil")
	}
}

// --- command-level profile wiring -------------------------------------------

// TestOverlayNew_ProfileWiringFromNoOverlays confirms the command appends the
// name to a profile that has no overlays key, producing exactly [name].
func TestOverlayNew_ProfileWiringFromNoOverlays(t *testing.T) {
	target := freshOverlayTarget(t, "profile: minimal\nmodules: [core]\npolicy_packs: []\n")
	out, err := runOverlayNewIn(t, target, "foo", "", "", false, "alpha")
	if err != nil {
		t.Fatalf("overlay new: %v\n%s", err, out)
	}
	got := readProfileOverlays(t, target)
	if len(got) != 1 || got[0] != "alpha" {
		t.Errorf("overlays=%v, want [alpha]", got)
	}
}

// TestOverlayNew_ProfileWiringAppendsToExisting confirms an existing overlays
// list gets the new name appended without disturbing prior entries.
func TestOverlayNew_ProfileWiringAppendsToExisting(t *testing.T) {
	target := freshOverlayTarget(t, "profile: minimal\nmodules: [core]\noverlays: [beta]\npolicy_packs: []\n")
	if _, err := runOverlayNewIn(t, target, "foo", "", "", false, "alpha"); err != nil {
		t.Fatal(err)
	}
	got := readProfileOverlays(t, target)
	if len(got) != 2 {
		t.Errorf("overlays=%v, want 2 entries (alpha + beta)", got)
	}
}

// TestOverlayNew_AlreadySelectedDoesNotDuplicate confirms re-running with a
// name already in overlays is a no-op on the profile (added=false, no dup).
func TestOverlayNew_AlreadySelectedDoesNotDuplicate(t *testing.T) {
	target := freshOverlayTarget(t, "profile: minimal\nmodules: [core]\noverlays: [alpha]\npolicy_packs: []\n")
	// Pre-create the pack dir so the command does not also try to scaffold it
	// (this test isolates the profile no-dup behavior; the pack dir already
	// existing surfaces as a pack-collision, which is the correct guard).
	packDir := filepath.Join(target, ".vh-agent-harness", "overlays", "alpha")
	if err := os.MkdirAll(packDir, 0o755); err != nil {
		t.Fatal(err)
	}
	out, err := runOverlayNewIn(t, target, "foo", "", "", false, "alpha")
	// The pack dir exists -> collision rejection (expected). The profile must be
	// untouched (no duplication).
	if err == nil {
		t.Fatalf("expected pack-collision rejection, got success\n%s", out)
	}
	got := readProfileOverlays(t, target)
	if len(got) != 1 || got[0] != "alpha" {
		t.Errorf("profile touched on collision: overlays=%v, want [alpha] unchanged", got)
	}
}

// --- strict dry-run ----------------------------------------------------------

// TestOverlayNew_DryRunWritesNothing confirms --dry-run prints the manifest +
// profile diff AND writes nothing to disk (no pack dir, no profile change).
func TestOverlayNew_DryRunWritesNothing(t *testing.T) {
	body := "profile: minimal\nmodules: [core]\noverlays: [beta]\npolicy_packs: []\n"
	target := freshOverlayTarget(t, body)
	out, err := runOverlayNewIn(t, target, "foo", "", "", true, "alpha")
	if err != nil {
		t.Fatalf("dry-run errored: %v\n%s", err, out)
	}
	// Output shape assertions.
	for _, want := range []string{"DRY RUN", "Would CREATE", "agents/foo.md", "Nothing was written", "alpha"} {
		if !strings.Contains(out, want) {
			t.Errorf("dry-run output missing %q\n--- output ---\n%s", want, out)
		}
	}
	// Profile diff shows before/after.
	if !strings.Contains(out, "overlays (before): [beta]") {
		t.Errorf("dry-run missing before-overlay diff\n%s", out)
	}
	if !strings.Contains(out, "overlays (after):  [alpha, beta]") && !strings.Contains(out, "overlays (after):  [beta, alpha]") {
		t.Errorf("dry-run missing after-overlay diff (sorted)\n%s", out)
	}
	// NOTHING written: no pack dir, profile byte-identical.
	if packExists(target, "alpha") {
		t.Errorf("dry-run created the pack dir")
	}
	got := readProfileOverlays(t, target)
	if len(got) != 1 || got[0] != "beta" {
		t.Errorf("dry-run mutated the profile: overlays=%v, want [beta]", got)
	}
}

// --- pack-dir / existing-file collision rejection ----------------------------

// TestOverlayNew_EmptyPackDirCollision confirms an existing (empty) pack dir is
// rejected as a collision with the "already exists" message and nothing is
// written.
func TestOverlayNew_EmptyPackDirCollision(t *testing.T) {
	target := freshOverlayTarget(t, "profile: minimal\nmodules: [core]\npolicy_packs: []\n")
	packDir := filepath.Join(target, ".vh-agent-harness", "overlays", "alpha")
	if err := os.MkdirAll(packDir, 0o755); err != nil {
		t.Fatal(err)
	}
	out, err := runOverlayNewIn(t, target, "foo", "", "", false, "alpha")
	if err == nil {
		t.Fatalf("expected collision rejection, got success\n%s", out)
	}
	if !strings.Contains(out, "already exists") {
		t.Errorf("collision output missing 'already exists'\n%s", out)
	}
}

// TestOverlayNew_ExistingFileConflict confirms a pack dir holding one of the
// plan files is rejected with the per-file conflict listing and nothing is
// overwritten.
func TestOverlayNew_ExistingFileConflict(t *testing.T) {
	target := freshOverlayTarget(t, "profile: minimal\nmodules: [core]\npolicy_packs: []\n")
	packDir := filepath.Join(target, ".vh-agent-harness", "overlays", "alpha")
	if err := os.MkdirAll(packDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Pre-create ONE plan file with sentinel content the command must NOT touch.
	sentinel := []byte("# HAND-WRITTEN — do not overwrite\n")
	if err := os.WriteFile(filepath.Join(packDir, "opencode-append.jsonc"), sentinel, 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := runOverlayNewIn(t, target, "foo", "", "", false, "alpha")
	if err == nil {
		t.Fatalf("expected conflict rejection, got success\n%s", out)
	}
	if !strings.Contains(out, "refusing to overwrite") || !strings.Contains(out, "opencode-append.jsonc") {
		t.Errorf("conflict output missing per-file listing\n%s", out)
	}
	// The sentinel file must be byte-identical (not overwritten).
	got, rerr := os.ReadFile(filepath.Join(packDir, "opencode-append.jsonc"))
	if rerr != nil {
		t.Fatalf("read sentinel: %v", rerr)
	}
	if string(got) != string(sentinel) {
		t.Errorf("conflict overwrote the existing file:\n got=%q\nwant=%q", got, sentinel)
	}
}

// --- unit-flag combinations --------------------------------------------------

// TestOverlayNew_AgentAloneCreatesAgentAndWiring confirms --agent alone creates
// the agent skeleton + the always-on files (opencode-append, permission-pack,
// callable-graph) with expected content, and NO command/skill skeleton. Covers
// advisories A-F1 (content on every planned file) + D-F4 (structural parse of
// opencode-append.jsonc) + D-F3 (removed a dead no-op loop that was here).
func TestOverlayNew_AgentAloneCreatesAgentAndWiring(t *testing.T) {
	target := freshOverlayTarget(t, "profile: minimal\nmodules: [core]\npolicy_packs: []\n")
	out, err := runOverlayNewIn(t, target, "foo", "", "", false, "alpha")
	if err != nil {
		t.Fatalf("overlay new: %v\n%s", err, out)
	}
	// A-F1: every planned file lands on disk with non-empty, expected content.
	type fileExpect struct {
		rel      string
		contains string // a substring the content MUST carry (sentinel of correctness)
	}
	for _, fe := range []fileExpect{
		{"agents/foo.md", "__UNIT_NAME__"}, // skeleton token must have been substituted away
		{"opencode-append.jsonc", "\"agent\""},
		{"permission-pack.jsonc", "\"agents\""},
		{"callable-graph-snippet.md", "<!--"},
	} {
		abs := filepath.Join(target, ".vh-agent-harness", "overlays", "alpha", filepath.FromSlash(fe.rel))
		body, rerr := os.ReadFile(abs)
		if rerr != nil {
			t.Errorf("A-F1: missing expected file overlays/alpha/%s: %v", fe.rel, rerr)
			continue
		}
		if len(body) == 0 {
			t.Errorf("A-F1: overlays/alpha/%s is empty", fe.rel)
		}
		// agents/foo.md: the __UNIT_NAME__ token must be GONE (substituted).
		if fe.rel == "agents/foo.md" && strings.Contains(string(body), fe.contains) {
			t.Errorf("A-F1: agents/foo.md still carries the __UNIT_NAME__ token (substitution failed)")
		}
		if fe.rel != "agents/foo.md" && !strings.Contains(string(body), fe.contains) {
			t.Errorf("A-F1: overlays/alpha/%s missing expected content %q", fe.rel, fe.contains)
		}
	}
	// No command/skill skeletons (explicit dir check, no dead loop).
	entries, _ := os.ReadDir(filepath.Join(target, ".vh-agent-harness", "overlays", "alpha"))
	for _, e := range entries {
		if e.Name() == "commands" || e.Name() == "skills" {
			t.Errorf("agent-only pack should not create %s/ dir, found %s", e.Name(), e.Name())
		}
	}
	// D-F4: parse the opencode-append.jsonc structurally and assert the agent
	// block + the permission.task.<agent> allow-injection keys exist (not just a
	// substring "foo":).
	appendBody, _ := os.ReadFile(filepath.Join(target, ".vh-agent-harness", "overlays", "alpha", "opencode-append.jsonc"))
	appendDoc, perr := parseGeneratedJSONC(appendBody)
	if perr != nil {
		t.Fatalf("D-F4: opencode-append.jsonc failed to parse as JSONC: %v\n%s", perr, appendBody)
	}
	agent, ok := appendDoc["agent"].(map[string]any)
	if !ok {
		t.Fatalf("D-F4: opencode-append.jsonc has no \"agent\" object")
	}
	fooBlock, ok := agent["foo"].(map[string]any)
	if !ok {
		t.Fatalf("D-F4: opencode-append.jsonc agent map has no \"foo\" entry (active wiring missing)")
	}
	if mode, _ := fooBlock["mode"].(string); mode != "subagent" {
		t.Errorf("D-F4: foo.mode=%q, want \"subagent\"", mode)
	}
	// The three orchestrators must each carry permission.task.foo = "allow".
	for _, orch := range []string{"build", "coordination", "project-coordinator"} {
		orchBlock, ok := agent[orch].(map[string]any)
		if !ok {
			t.Errorf("D-F4: opencode-append.jsonc missing orchestrator %q", orch)
			continue
		}
		perm, _ := orchBlock["permission"].(map[string]any)
		task, _ := perm["task"].(map[string]any)
		if got, _ := task["foo"].(string); got != "allow" {
			t.Errorf("D-F4: agent.%s.permission.task.foo=%q, want \"allow\"", orch, got)
		}
	}
}

// TestOverlayNew_AgentAndCommand confirms --agent --command creates both unit
// skeletons in one pack.
func TestOverlayNew_AgentAndCommand(t *testing.T) {
	target := freshOverlayTarget(t, "profile: minimal\nmodules: [core]\npolicy_packs: []\n")
	if _, err := runOverlayNewIn(t, target, "foo", "bar", "", false, "alpha"); err != nil {
		t.Fatal(err)
	}
	if !packFileExists(target, "alpha", "agents/foo.md") {
		t.Errorf("missing agents/foo.md")
	}
	if !packFileExists(target, "alpha", "commands/bar.md") {
		t.Errorf("missing commands/bar.md")
	}
}

// TestOverlayNew_NoUnitFlagWarnsAndMinimalPack confirms passing no unit flag
// still creates the pack (the always-on files) and prints a warning that no
// unit was requested.
func TestOverlayNew_NoUnitFlagWarnsAndMinimalPack(t *testing.T) {
	target := freshOverlayTarget(t, "profile: minimal\nmodules: [core]\npolicy_packs: []\n")
	out, err := runOverlayNewIn(t, target, "", "", "", false, "alpha")
	if err != nil {
		t.Fatalf("no-unit overlay new: %v\n%s", err, out)
	}
	if !strings.Contains(out, "warning") || !strings.Contains(out, "minimal pack") {
		t.Errorf("no-unit output missing minimal-pack warning\n%s", out)
	}
	// Still creates the always-on files (opencode-append is a no-op shell).
	if !packFileExists(target, "alpha", "opencode-append.jsonc") {
		t.Errorf("no-unit pack missing opencode-append.jsonc")
	}
	// The no-op opencode-append must be a valid empty JSONC object.
	body, _ := os.ReadFile(filepath.Join(target, ".vh-agent-harness", "overlays", "alpha", "opencode-append.jsonc"))
	if !strings.Contains(string(body), "{") {
		t.Errorf("no-unit opencode-append.jsonc is not a JSON object")
	}
}

// --- name validation ---------------------------------------------------------

// TestOverlayNew_RejectsBadPackName confirms an invalid pack name is rejected
// before anything is written. (packExists is not asserted here because some
// invalid names like ".." join-resolve to an existing parent dir, making the
// existence check meaningless; the err != nil + error-message checks are the
// authoritative rejection evidence.)
func TestOverlayNew_RejectsBadPackName(t *testing.T) {
	target := freshOverlayTarget(t, "profile: minimal\nmodules: [core]\npolicy_packs: []\n")
	for _, bad := range []string{"UPPER", "has space", "trailing/", ".leadingdot", "-leadingdash", ".."} {
		out, err := runOverlayNewIn(t, target, "foo", "", "", false, bad)
		if err == nil {
			t.Errorf("bad pack name %q should be rejected\n%s", bad, out)
		}
		if !strings.Contains(out, "error:") {
			t.Errorf("bad pack name %q: output missing error:\n%s", bad, out)
		}
	}
}

// --- target guard ------------------------------------------------------------

// TestOverlayNew_RejectsMissingHarnessDir confirms the command refuses when
// .vh-agent-harness/ is absent (points the operator to install).
func TestOverlayNew_RejectsMissingHarnessDir(t *testing.T) {
	target := t.TempDir() // NO .vh-agent-harness/
	out, err := runOverlayNewIn(t, target, "foo", "", "", false, "alpha")
	if err == nil {
		t.Fatalf("expected missing-harness rejection, got success\n%s", out)
	}
	if !strings.Contains(out, "no .vh-agent-harness/") {
		t.Errorf("output should point to install\n%s", out)
	}
}

// --- B-F1 regression: permission-pack gate/gateExempt contract ----------------

// TestOverlayNew_PermissionPackIsValidForActiveAgent is the regression test for
// B-F1. It generates an `--agent <a>` pack and asserts the generated
// permission-pack.jsonc is valid for an ACTIVE pack — specifically that the
// gate/gateExempt contract holds: an agent marked gateExempt: true must NOT
// carry a `gate` key in its `location` block.
//
// Why this matters: `overlay new` also appends the pack to `overlays:` (making
// it ACTIVE). The next `vh-agent-harness update` materializes the pack to
// .opencode/sys-scripts/permission-packs/<pack>.jsonc and the Go-native emitter
// (internal/permconfig) resolves it into permission blocks. The emitter's
// resolveRules() copies def.location (including any `gate` key) into its
// location table and adds the agent to the gate-exempt set when gateExempt is
// true; validate() then FAILS with
//
//	"LOCATION_RULES.<agent> must NOT have a gate key (gate deny bleeds into
//	 committer subagent)"
//
// if a gateExempt agent carries a gate decision. The crash aborts the ENTIRE
// permission render (no agent's blocks update), so a single bad pack bricks the
// repo's permission system. See internal/permconfig/{tables.go,emit.go}.
//
// This test encodes the contract at the source (the generated JSONC) so it runs
// fast and deterministically without spawning node. Driving the live JS path is
// covered by the throwaway e2e (see the B-F1 closeout note in backlog.md).
func TestOverlayNew_PermissionPackIsValidForActiveAgent(t *testing.T) {
	target := freshOverlayTarget(t, "profile: minimal\nmodules: [core]\npolicy_packs: []\n")
	if out, err := runOverlayNewIn(t, target, "foo", "", "", false, "alpha"); err != nil {
		t.Fatalf("overlay new --agent foo: %v\n%s", err, out)
	}
	body, err := os.ReadFile(filepath.Join(target, ".vh-agent-harness", "overlays", "alpha", "permission-pack.jsonc"))
	if err != nil {
		t.Fatalf("read permission-pack.jsonc: %v", err)
	}
	doc, perr := parseGeneratedJSONC(body)
	if perr != nil {
		t.Fatalf("permission-pack.jsonc failed to parse as JSONC: %v\n%s", perr, body)
	}
	agents, ok := doc["agents"].(map[string]any)
	if !ok {
		t.Fatalf("permission-pack.jsonc has no \"agents\" object")
	}
	fooDef, ok := agents["foo"].(map[string]any)
	if !ok {
		t.Fatalf("permission-pack.jsonc has no agents.foo entry")
	}
	// The scaffolded agent is a committer-delegator: gateExempt MUST be true.
	if ge, _ := fooDef["gateExempt"].(bool); !ge {
		t.Errorf("agents.foo.gateExempt = %v, want true (scaffolded committer-delegator)", ge)
	}
	// B-F1 core: a gateExempt agent must NOT carry a gate key in location.
	loc, ok := fooDef["location"].(map[string]any)
	if !ok {
		t.Fatalf("agents.foo has no location object")
	}
	if _, has := loc["gate"]; has {
		t.Errorf("B-F1 REGRESSION: agents.foo.location has a \"gate\" key while gateExempt=true — " +
			"validateRules() will fail with \"LOCATION_RULES.foo must NOT have a gate key " +
			"(gate deny bleeds into committer subagent)\". Omit gate for gateExempt agents.")
	}
	// And the required non-gate command-group decisions must all be present +
	// valid (wildcard/readonly/git_readonly/harness), mirroring validateRules.
	for _, key := range []string{"wildcard", "readonly", "git_readonly", "harness"} {
		v, has := loc[key]
		if !has {
			t.Errorf("agents.foo.location missing required key %q", key)
			continue
		}
		dec, _ := v.(string)
		switch dec {
		case "allow", "ask", "deny":
		default:
			t.Errorf("agents.foo.location.%s = %q, want one of allow/ask/deny", key, dec)
		}
	}
	// task must carry the committer allow (the delegator contract).
	if task, ok := fooDef["task"].(map[string]any); !ok {
		t.Errorf("agents.foo missing task object")
	} else if got, _ := task["committer"].(string); got != "allow" {
		t.Errorf("agents.foo.task.committer = %q, want \"allow\"", got)
	}
}

// --- B-F1: no-agent pack must not declare a fake agent -----------------------

// TestOverlayNew_PermissionPackNoAgentIsHarmless confirms that with NO --agent,
// the generated permission-pack.jsonc still parses and (because the agent name
// is the literal <name> placeholder) does not accidentally introduce a real
// roster entry that would fail validateRules. The pack is appended to overlays
// regardless, so the file must remain valid JSONC.
func TestOverlayNew_PermissionPackNoAgentIsHarmless(t *testing.T) {
	target := freshOverlayTarget(t, "profile: minimal\nmodules: [core]\npolicy_packs: []\n")
	if out, err := runOverlayNewIn(t, target, "", "", "", false, "alpha"); err != nil {
		t.Fatalf("overlay new (no unit): %v\n%s", err, out)
	}
	body, err := os.ReadFile(filepath.Join(target, ".vh-agent-harness", "overlays", "alpha", "permission-pack.jsonc"))
	if err != nil {
		t.Fatalf("read permission-pack.jsonc: %v", err)
	}
	if _, perr := parseGeneratedJSONC(body); perr != nil {
		t.Fatalf("no-agent permission-pack.jsonc is not valid JSONC: %v\n%s", perr, body)
	}
}

// parseGeneratedJSONC strips full-line `//` comments and parses the remainder as
// JSON. It is deliberately minimal: the scaffolder-generated permission-pack and
// opencode-append files use ONLY full-line leading `//` comments (no inline
// comments, no block comments), so a line-based strip suffices and avoids
// pulling in a JSONC dependency. Not a general JSONC parser.
func parseGeneratedJSONC(raw []byte) (map[string]any, error) {
	var kept []string
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "//") {
			continue
		}
		kept = append(kept, line)
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(strings.Join(kept, "\n")), &out); err != nil {
		return nil, err
	}
	return out, nil
}
