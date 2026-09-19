package cli

// capability_coherence_test.go — regression fixtures + O4 positive-presence
// tests for the git-routing/gated-commit capability coherence work (task
// git-routing-capability-coherence: O2 diagnostics + O5 conditional wording +
// O4 selected-cluster assertions).
//
// Fixture map (task card Phase 2):
//   - minimal/default coherent inactivity ......... TestCoherence_MinimalCoherentUnselected
//   - selected success ............................ TestCoherence_SupervisedSelectedWired
//   - selected-but-missing-agent-block ............ TestCoherence_SelectedMissingAgentBlock
//   - selected-but-missing-required-caller-edge ... TestCoherence_SelectedMissingCallerEdge
//   - optional overlay absent-target still passes . TestCoherence_GracefulDegradationStaysHealthy
//   - unknown/unreadable config ................... TestCoherence_UnknownConfig
//   - inactive residue ............................ TestCoherence_InactiveResidue
//   - preserved edits (routing-doc note) .......... TestCoherence_PreservedRoutingWordingNoted
//   - no-write previews ........................... TestCoherence_DryRunNoWriteNoWarning
//   - pure classifier unit table ..................TestClassifyGatedCommitCoherence_Table
//
// O5 wording-projection fixtures live at the bottom (minimal vs supervised
// rendered guidance markers + denial payload conditional sentence).

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	corpus "github.com/vhqtvn/vh-agent-harness"
)

// rewriteOpencodeJSON parses the rendered opencode.jsonc (canonical emitter
// output is strict JSON), applies mutate to the parsed document, and writes it
// back. Fixture surgery helper for the doctor LIVE-check tests.
func rewriteOpencodeJSON(t *testing.T, root string, mutate func(doc map[string]any)) {
	t.Helper()
	p := filepath.Join(root, opencodeJSONCRel)
	raw, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read opencode.jsonc: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("unmarshal opencode.jsonc: %v", err)
	}
	mutate(doc)
	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		t.Fatalf("remarshal opencode.jsonc: %v", err)
	}
	if err := os.WriteFile(p, out, 0o644); err != nil {
		t.Fatalf("write opencode.jsonc: %v", err)
	}
}

// coherenceLine extracts the capability-coherence RESULT line (the one after
// the section header) from doctor output so assertions are scoped to THIS
// check rather than the whole report.
func coherenceLine(t *testing.T, doctorOut string) string {
	t.Helper()
	lines := strings.Split(doctorOut, "\n")
	for i, line := range lines {
		if strings.TrimSpace(line) == "capability-coherence:" && i+1 < len(lines) {
			return strings.TrimSpace(lines[i+1])
		}
	}
	t.Fatalf("doctor output lacks a capability-coherence result line:\n%s", doctorOut)
	return ""
}

// --- fixture: minimal/default coherent inactivity ---------------------------

// TestCoherence_MinimalCoherentUnselected pins the NO-NAG contract: a minimal
// profile (capability unselected, no gated wiring in the config) reports a
// PASS coherence line — never a WARN, never a FAIL. Valid inactivity is a
// healthy state, not a diagnostic finding.
func TestCoherence_MinimalCoherentUnselected(t *testing.T) {
	root := t.TempDir()
	seamInstallInto(t, root)
	writeProfile(t, root, "profile: minimal\nfeatures:\n  backlog: true\noverlays: []\npolicy_packs: []\n")
	if _, err := seamUpdateOut(t, root); err != nil {
		t.Fatalf("update minimal: %v", err)
	}
	out := seamDoctorOut(t, root)
	line := coherenceLine(t, out)
	if !strings.Contains(line, "PASS") {
		t.Errorf("minimal coherent-unselected must be PASS (no nag); got:\n%s", line)
	}
	if !strings.Contains(line, "coherent inactivity") {
		t.Errorf("minimal coherent line must name coherent inactivity; got:\n%s", line)
	}
	if strings.Contains(line, "WARN") || strings.Contains(line, "FAIL") {
		t.Errorf("minimal coherent-unselected must never WARN/FAIL; got:\n%s", line)
	}
	// The whole report must stay HEALTHY (the check adds no new failure mode).
	if !strings.Contains(out, "result: HEALTHY") {
		t.Errorf("doctor must stay HEALTHY on coherent minimal; got:\n%s", out)
	}
}

// --- fixture: selected success (O4 positive presence) ------------------------

// TestCoherence_SupervisedSelectedWired pins the selected-cluster contract
// (O4-revised): under the supervised preset every core/gated-commit agent
// block AND every required core caller task edge is present in the emitted
// config, and doctor's coherence line reports the selected-wired PASS.
func TestCoherence_SupervisedSelectedWired(t *testing.T) {
	root := t.TempDir()
	seamInstallInto(t, root)
	writeProfile(t, root, "profile: supervised\nfeatures:\n  backlog: true\noverlays: []\npolicy_packs: []\n")
	if _, err := seamUpdateOut(t, root); err != nil {
		t.Fatalf("update supervised: %v", err)
	}

	// O4 positive-presence assertions on the emitted config itself.
	rendered := parseRenderedAgents(t, root)
	assertAgentsPresent(t, rendered, gatedCommitAgents)
	edges := parseRenderedTaskEdges(t, root)
	for _, caller := range []string{"build", "coordination", "project-coordinator"} {
		for _, target := range []string{"committer", "commit-message", "commit-reviewer"} {
			if !edges[caller][target] {
				t.Errorf("required caller edge %s -> %s must be present when core/gated-commit is selected", caller, target)
			}
		}
	}
	if !edges["docs-steward"]["committer"] {
		t.Errorf("required caller edge docs-steward -> committer must be present when core/gated-commit is selected")
	}

	out := seamDoctorOut(t, root)
	line := coherenceLine(t, out)
	if !strings.Contains(line, "PASS") {
		t.Errorf("supervised selected+wired must be PASS; got:\n%s", line)
	}
	if !strings.Contains(line, "selected") {
		t.Errorf("supervised coherence line must name selection; got:\n%s", line)
	}
	// H2: the selected PASS line carries the unconditional restart caveat
	// (advisory file-level view; never a claim about a running process).
	if !strings.Contains(line, "restart opencode") {
		t.Errorf("selected-wired line must carry the restart caveat (H2); got:\n%s", line)
	}
	if !strings.Contains(out, "result: HEALTHY") {
		t.Errorf("doctor must stay HEALTHY on coherent supervised; got:\n%s", out)
	}
}

// --- fixture: selected-but-missing-agent-block -------------------------------

// TestCoherence_SelectedMissingAgentBlock doctors the LIVE config (delete the
// committer agent block) and asserts doctor reports a WARN naming the missing
// block — advisory, never FAIL.
func TestCoherence_SelectedMissingAgentBlock(t *testing.T) {
	root := t.TempDir()
	seamInstallInto(t, root)
	writeProfile(t, root, "profile: supervised\nfeatures:\n  backlog: true\noverlays: []\npolicy_packs: []\n")
	if _, err := seamUpdateOut(t, root); err != nil {
		t.Fatalf("update supervised: %v", err)
	}
	rewriteOpencodeJSON(t, root, func(doc map[string]any) {
		agents := doc["agent"].(map[string]any)
		delete(agents, "committer")
	})
	out := seamDoctorOut(t, root)
	line := coherenceLine(t, out)
	if !strings.Contains(line, "WARN") {
		t.Errorf("selected-but-missing-agent-block must be WARN; got:\n%s", line)
	}
	if !strings.Contains(line, "committer") {
		t.Errorf("WARN must name the missing agent block; got:\n%s", line)
	}
	if !strings.Contains(line, "restart opencode") {
		t.Errorf("WARN must carry the restart caveat (H2); got:\n%s", line)
	}
	if strings.Contains(line, "FAIL") {
		t.Errorf("coherence findings must never FAIL (no new blocking gate); got:\n%s", line)
	}
}

// --- fixture: selected-but-missing-required-caller-edge ----------------------

// TestCoherence_SelectedMissingCallerEdge deletes ONLY the build -> committer
// task edge from the live config and asserts a WARN naming that caller edge.
func TestCoherence_SelectedMissingCallerEdge(t *testing.T) {
	root := t.TempDir()
	seamInstallInto(t, root)
	writeProfile(t, root, "profile: supervised\nfeatures:\n  backlog: true\noverlays: []\npolicy_packs: []\n")
	if _, err := seamUpdateOut(t, root); err != nil {
		t.Fatalf("update supervised: %v", err)
	}
	rewriteOpencodeJSON(t, root, func(doc map[string]any) {
		agents := doc["agent"].(map[string]any)
		build := agents["build"].(map[string]any)
		perm := build["permission"].(map[string]any)
		task := perm["task"].(map[string]any)
		delete(task, "committer")
	})
	out := seamDoctorOut(t, root)
	line := coherenceLine(t, out)
	if !strings.Contains(line, "WARN") {
		t.Errorf("selected-but-missing-caller-edge must be WARN; got:\n%s", line)
	}
	if !strings.Contains(line, "build -> committer") {
		t.Errorf("WARN must name the missing caller edge; got:\n%s", line)
	}
}

// --- fixture: optional overlay absent-target still passes --------------------

// TestCoherence_GracefulDegradationStaysHealthy pins the intentional
// present-agent filter behavior from the coherence side: a profile selecting
// ONLY core/debate (gated-commit unselected) drops gated agents and their
// edges (graceful degradation — no emitter hard error, no coherence warning).
func TestCoherence_GracefulDegradationStaysHealthy(t *testing.T) {
	root := t.TempDir()
	seamInstallInto(t, root)
	writeProfile(t, root, "profile: minimal\nfeatures:\n  backlog: true\noverlays: []\npolicy_packs: []\ncapabilities:\n  - core/debate\n")
	if _, err := seamUpdateOut(t, root); err != nil {
		t.Fatalf("update minimal+debate: %v", err)
	}
	rendered := parseRenderedAgents(t, root)
	assertAgentsAbsent(t, rendered, gatedCommitAgents)
	assertAgentsPresent(t, rendered, debateAgents)
	out := seamDoctorOut(t, root)
	line := coherenceLine(t, out)
	if !strings.Contains(line, "PASS") {
		t.Errorf("graceful-degradation render must be a coherent PASS; got:\n%s", line)
	}
	if !strings.Contains(out, "result: HEALTHY") {
		t.Errorf("doctor must stay HEALTHY on minimal+debate; got:\n%s", out)
	}
}

// --- fixture: unknown/unreadable config --------------------------------------

// TestCoherence_UnknownConfig corrupts opencode.jsonc and asserts the check
// degrades to INFO (coherence unknown) without panicking — other checks own
// the unparseable-config failure surface.
func TestCoherence_UnknownConfig(t *testing.T) {
	root := t.TempDir()
	seamInstallInto(t, root)
	writeProfile(t, root, "profile: supervised\nfeatures:\n  backlog: true\noverlays: []\npolicy_packs: []\n")
	if _, err := seamUpdateOut(t, root); err != nil {
		t.Fatalf("update supervised: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, opencodeJSONCRel), []byte("{ not json"), 0o644); err != nil {
		t.Fatalf("corrupt opencode.jsonc: %v", err)
	}
	out := seamDoctorOut(t, root)
	line := coherenceLine(t, out)
	if !strings.Contains(line, "INFO") {
		t.Errorf("unreadable config must be INFO (coherence unknown); got:\n%s", line)
	}
	if strings.Contains(line, "FAIL") || strings.Contains(line, "WARN") {
		t.Errorf("unreadable config must not WARN/FAIL on the coherence line; got:\n%s", line)
	}
}

// --- fixture: inactive residue ------------------------------------------------

// TestCoherence_InactiveResidue hand-wires a committer block into a minimal
// (unselected) config and asserts an INFO residue observation — not a warning,
// not a failure.
func TestCoherence_InactiveResidue(t *testing.T) {
	root := t.TempDir()
	seamInstallInto(t, root)
	writeProfile(t, root, "profile: minimal\nfeatures:\n  backlog: true\noverlays: []\npolicy_packs: []\n")
	if _, err := seamUpdateOut(t, root); err != nil {
		t.Fatalf("update minimal: %v", err)
	}
	rewriteOpencodeJSON(t, root, func(doc map[string]any) {
		agents := doc["agent"].(map[string]any)
		agents["committer"] = map[string]any{
			"description": "hand-wired residue fixture",
			"mode":        "subagent",
			"permission": map[string]any{
				"task": map[string]any{"*": "deny"},
			},
		}
	})
	out := seamDoctorOut(t, root)
	line := coherenceLine(t, out)
	if !strings.Contains(line, "INFO") {
		t.Errorf("unselected-but-present wiring must be INFO residue; got:\n%s", line)
	}
	if !strings.Contains(line, "committer") {
		t.Errorf("residue INFO must name the residue agent; got:\n%s", line)
	}
}

// --- fixture: preserved edits (routing-doc update note) -----------------------

// TestCoherence_PreservedRoutingWordingNoted pins the H1 update-note: when the
// adopter's local edit to the routing doc is PRESERVED across an update
// (managed-diverged, never clobbered), the seam prints the advisory note
// calling out the capability-conditional wording change, and the preserved
// bytes survive byte-for-byte.
func TestCoherence_PreservedRoutingWordingNoted(t *testing.T) {
	root := t.TempDir()
	seamInstallInto(t, root)
	writeProfile(t, root, "profile: minimal\nfeatures:\n  backlog: true\noverlays: []\npolicy_packs: []\n")
	if _, err := seamUpdateOut(t, root); err != nil {
		t.Fatalf("update minimal: %v", err)
	}

	const rel = routingDocLiveRel
	edited := []byte("# Git Execution Routing Rule\n\nMY LOCAL EDIT — must survive update.\n")
	if err := os.WriteFile(filepath.Join(root, rel), edited, 0o644); err != nil {
		t.Fatalf("hand-edit routing doc: %v", err)
	}

	var stderr string
	var err2 error
	stderr = captureStderr(t, func() {
		_, err2 = seamUpdateOut(t, root)
	})
	if err2 != nil {
		t.Fatalf("update with preserved routing doc must succeed: %v", err2)
	}
	if !strings.Contains(stderr, "PRESERVED with your local edits") {
		t.Errorf("preserved routing doc must trigger the wording-change note; stderr:\n%s", stderr)
	}
	if !strings.Contains(stderr, "Capability condition") {
		t.Errorf("note must point at the new Capability condition wording; stderr:\n%s", stderr)
	}
	got, rerr := os.ReadFile(filepath.Join(root, rel))
	if rerr != nil {
		t.Fatalf("read routing doc after update: %v", rerr)
	}
	if string(got) != string(edited) {
		t.Errorf("preserved routing doc was NOT byte-preserved (clobbered)")
	}
}

// --- fixture: no-write previews ------------------------------------------------

// TestCoherence_ProspectiveWarningFiresOnIncoherentStaging exercises the
// POSITIVE branch of warnIfGatedCommitIncoherent directly (the commit-review
// F-A1/F-C1 defer): a supervised repo whose STAGED config is missing the
// committer block must print the prospective WARNING, and a minimal repo
// whose staged config carries residue wiring must print the residue WARNING.
// The silent-coherent path is covered end-to-end by
// TestCoherence_DryRunNoWriteNoWarning.
func TestCoherence_ProspectiveWarningFiresOnIncoherentStaging(t *testing.T) {
	// Selected-but-missing-block: supervised repo, staged config without the
	// committer agent block.
	root := t.TempDir()
	seamInstallInto(t, root)
	writeProfile(t, root, "profile: supervised\nfeatures:\n  backlog: true\noverlays: []\npolicy_packs: []\n")
	if _, err := seamUpdateOut(t, root); err != nil {
		t.Fatalf("update supervised: %v", err)
	}
	staging := t.TempDir()
	stagingCfg := filepath.Join(staging, opencodeJSONCRel)
	if err := os.MkdirAll(staging, 0o755); err != nil {
		t.Fatalf("mkdir staging: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(root, opencodeJSONCRel))
	if err != nil {
		t.Fatalf("read live config: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("unmarshal live config: %v", err)
	}
	delete(doc["agent"].(map[string]any), "committer")
	doctored, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		t.Fatalf("remarshal doctored config: %v", err)
	}
	if err := os.WriteFile(stagingCfg, doctored, 0o644); err != nil {
		t.Fatalf("write staged config: %v", err)
	}
	stderr := captureStderr(t, func() {
		warnIfGatedCommitIncoherent(root, staging)
	})
	for _, want := range []string{"PROSPECTIVE", "core/gated-commit", "committer", "restart opencode"} {
		if !strings.Contains(stderr, want) {
			t.Errorf("prospective warning must contain %q; stderr:\n%s", want, stderr)
		}
	}

	// Unselected-residue: minimal repo, staged config WITH a hand-added
	// committer block.
	root2 := t.TempDir()
	seamInstallInto(t, root2)
	writeProfile(t, root2, "profile: minimal\nfeatures:\n  backlog: true\noverlays: []\npolicy_packs: []\n")
	if _, err := seamUpdateOut(t, root2); err != nil {
		t.Fatalf("update minimal: %v", err)
	}
	staging2 := t.TempDir()
	if err := os.MkdirAll(staging2, 0o755); err != nil {
		t.Fatalf("mkdir staging2: %v", err)
	}
	raw2, err := os.ReadFile(filepath.Join(root2, opencodeJSONCRel))
	if err != nil {
		t.Fatalf("read live config 2: %v", err)
	}
	var doc2 map[string]any
	if err := json.Unmarshal(raw2, &doc2); err != nil {
		t.Fatalf("unmarshal live config 2: %v", err)
	}
	doc2["agent"].(map[string]any)["committer"] = map[string]any{
		"mode": "subagent",
		"permission": map[string]any{
			"task": map[string]any{"*": "deny"},
		},
	}
	doctored2, err := json.MarshalIndent(doc2, "", "  ")
	if err != nil {
		t.Fatalf("remarshal doctored config 2: %v", err)
	}
	if err := os.WriteFile(filepath.Join(staging2, opencodeJSONCRel), doctored2, 0o644); err != nil {
		t.Fatalf("write staged config 2: %v", err)
	}
	stderr2 := captureStderr(t, func() {
		warnIfGatedCommitIncoherent(root2, staging2)
	})
	for _, want := range []string{"PROSPECTIVE", "NOT selected", "committer", "emitter/template regression"} {
		if !strings.Contains(stderr2, want) {
			t.Errorf("residue prospective warning must contain %q; stderr:\n%s", want, stderr2)
		}
	}
}

// TestCoherence_DryRunNoWriteNoWarning asserts update --dry-run on a coherent
// repo (a) prints NO coherence warning (healthy renders stay quiet) and
// (b) leaves the live config byte-identical (pure preview).
func TestCoherence_DryRunNoWriteNoWarning(t *testing.T) {
	root := t.TempDir()
	seamInstallInto(t, root)
	writeProfile(t, root, "profile: supervised\nfeatures:\n  backlog: true\noverlays: []\npolicy_packs: []\n")
	if _, err := seamUpdateOut(t, root); err != nil {
		t.Fatalf("update supervised: %v", err)
	}
	before, err := os.ReadFile(filepath.Join(root, opencodeJSONCRel))
	if err != nil {
		t.Fatalf("read config before dry-run: %v", err)
	}

	withDryRun(t, true)
	var out string
	var uerr error
	stderr := captureStderr(t, func() {
		out, uerr = runUpdateTarget(t, root)
	})
	if uerr != nil {
		t.Fatalf("dry-run must preview cleanly; got %v (out=%q)", uerr, out)
	}
	if strings.Contains(stderr, "capability/wiring mismatch") ||
		strings.Contains(stderr, "gated-commit wiring present") {
		t.Errorf("coherent dry-run must print NO coherence warning; stderr:\n%s", stderr)
	}
	after, err := os.ReadFile(filepath.Join(root, opencodeJSONCRel))
	if err != nil {
		t.Fatalf("read config after dry-run: %v", err)
	}
	if string(before) != string(after) {
		t.Errorf("dry-run must leave opencode.jsonc byte-identical (pure preview)")
	}
}

// --- pure classifier unit table -------------------------------------------------

func TestClassifyGatedCommitCoherence_Table(t *testing.T) {
	provides := []string{"commit-message", "commit-reviewer", "committer"}

	makeDoc := func(blocks map[string]any, edges map[string]map[string]string) map[string]any {
		agents := map[string]any{}
		for name, blk := range blocks {
			agents[name] = blk
		}
		for caller, tgts := range edges {
			blk, ok := agents[caller].(map[string]any)
			if !ok {
				continue // absent caller: never resurrect a deleted block
			}
			// encoding/json produces map[string]any for nested objects; mirror
			// that here so the classifier's type assertions see real shapes.
			taskAny := map[string]any{}
			for k, v := range tgts {
				taskAny[k] = v
			}
			blk["permission"] = map[string]any{"task": taskAny}
		}
		return map[string]any{"agent": agents}
	}
	blk := func() map[string]any { return map[string]any{"permission": map[string]any{"task": map[string]any{}}} }

	// unselected + nothing present → coherent
	got := classifyGatedCommitCoherence(false, provides, makeDoc(nil, nil))
	if got.State != cohUnselectedClean {
		t.Errorf("unselected/clean: got %s want %s", got.State, cohUnselectedClean)
	}
	// unselected + residue → residue
	got = classifyGatedCommitCoherence(false, provides, makeDoc(map[string]any{"committer": blk()}, nil))
	if got.State != cohResidue || len(got.ResidueBlocks) != 1 || got.ResidueBlocks[0] != "committer" {
		t.Errorf("unselected/residue: got %+v", got)
	}
	// selected + all blocks + edges → wired. fullEdges covers every
	// CoreTaskRules entry whose target is a gated-commit Provides agent:
	// orchestrator + docs-steward inbound edges AND the intra-cluster edges
	// (committer -> commit-reviewer, commit-message -> commit-reviewer).
	fullEdges := map[string]map[string]string{
		"build":               {"committer": "allow", "commit-message": "allow", "commit-reviewer": "allow"},
		"coordination":        {"committer": "allow", "commit-message": "allow", "commit-reviewer": "allow"},
		"project-coordinator": {"committer": "allow", "commit-message": "allow", "commit-reviewer": "allow"},
		"docs-steward":        {"committer": "allow"},
		"committer":           {"commit-reviewer": "allow"},
		"commit-message":      {"commit-reviewer": "allow"},
	}
	blocks := map[string]any{
		"commit-message": blk(), "commit-reviewer": blk(), "committer": blk(),
		"build": blk(), "coordination": blk(), "project-coordinator": blk(), "docs-steward": blk(),
	}
	got = classifyGatedCommitCoherence(true, provides, makeDoc(blocks, fullEdges))
	if got.State != cohSelectedWired {
		t.Errorf("selected/wired: got %s want %s (finding=%+v)", got.State, cohSelectedWired, got)
	}
	// selected + missing block → missing-blocks (edge to it not double-reported)
	brokenBlocks := map[string]any{
		"commit-message": blk(), "commit-reviewer": blk(),
		"build": blk(), "coordination": blk(), "project-coordinator": blk(), "docs-steward": blk(),
	}
	got = classifyGatedCommitCoherence(true, provides, makeDoc(brokenBlocks, fullEdges))
	if got.State != cohMissingBlocks || got.MissingBlocks[0] != "committer" {
		t.Errorf("selected/missing-block: got %+v", got)
	}
	for _, e := range got.MissingEdges {
		if strings.HasSuffix(e, "committer") {
			t.Errorf("missing target block must not be double-reported as missing edges; got %+v", got.MissingEdges)
		}
	}
	// selected + all blocks but build->committer edge dropped → missing-edges
	dropEdge := map[string]map[string]string{}
	for k, v := range fullEdges {
		dropEdge[k] = map[string]string{}
		for t2, d := range v {
			dropEdge[k][t2] = d
		}
	}
	delete(dropEdge["build"], "committer")
	got = classifyGatedCommitCoherence(true, provides, makeDoc(blocks, dropEdge))
	if got.State != cohMissingEdges || got.MissingEdges[0] != "build -> committer" {
		t.Errorf("selected/missing-edge: got %+v", got)
	}
	// caller block absent (optional caller) → its edge is NOT required
	delete(fullEdges, "docs-steward")
	delete(blocks, "docs-steward")
	got = classifyGatedCommitCoherence(true, provides, makeDoc(blocks, fullEdges))
	if got.State != cohSelectedWired {
		t.Errorf("absent optional caller must not require its edge: got %+v", got)
	}
}

// --- O5 wording projection: rendered guidance (minimal vs supervised) ----------

// gitRoutingUnavailableMarkers MUST appear in the rendered build/coordination
// prompts when core/gated-commit is NOT selected (honest unavailable-branch
// wording: stop, preserve, report, request activation/operator handling).
var gitRoutingUnavailableMarkers = []string{
	"## Git commit routing (capability not available)",
	"do not probe",
	"preserve the work",
	"report the missing route",
	"core/gated-commit",
}

// gitRoutingAvailableMarkers MUST appear when core/gated-commit IS selected
// (the reviewed committer route, unchanged from the pre-O5 wording).
var gitRoutingAvailableMarkers = []string{
	"## Git commit routing (capability available)",
	"committer",
}

func TestO5_RenderedGuidance_MinimalVsSupervised(t *testing.T) {
	minimal := t.TempDir()
	seamInstallInto(t, minimal)
	writeProfile(t, minimal, "profile: minimal\nfeatures:\n  backlog: true\noverlays: []\npolicy_packs: []\n")
	if _, err := seamUpdateOut(t, minimal); err != nil {
		t.Fatalf("update minimal: %v", err)
	}

	supervised := t.TempDir()
	seamInstallInto(t, supervised)
	writeProfile(t, supervised, "profile: supervised\nfeatures:\n  backlog: true\noverlays: []\npolicy_packs: []\n")
	if _, err := seamUpdateOut(t, supervised); err != nil {
		t.Fatalf("update supervised: %v", err)
	}

	// Template-conditional agent prompts: the unavailable branch renders ONLY
	// under minimal; the available branch ONLY under supervised.
	for _, agent := range []string{"build", "coordination"} {
		minimalPrompt := readRenderedAgentPrompt(t, minimal, agent)
		for _, m := range gitRoutingUnavailableMarkers {
			if !strings.Contains(minimalPrompt, m) {
				t.Errorf("minimal %s.md must contain unavailable-branch marker %q", agent, m)
			}
		}
		if strings.Contains(minimalPrompt, "## Git commit routing (capability available)") {
			t.Errorf("minimal %s.md must NOT render the available branch", agent)
		}

		supPrompt := readRenderedAgentPrompt(t, supervised, agent)
		for _, m := range gitRoutingAvailableMarkers {
			if !strings.Contains(supPrompt, m) {
				t.Errorf("supervised %s.md must contain available-branch marker %q", agent, m)
			}
		}
		if strings.Contains(supPrompt, "## Git commit routing (capability not available)") {
			t.Errorf("supervised %s.md must NOT render the unavailable branch", agent)
		}
	}

	// Static conditional wording renders identically on BOTH profiles (no
	// template gates on these surfaces — static prose, no leaked syntax).
	for _, root := range []string{minimal, supervised} {
		doc, err := os.ReadFile(filepath.Join(root, routingDocLiveRel))
		if err != nil {
			t.Fatalf("read rendered routing doc: %v", err)
		}
		for _, m := range []string{"## Capability condition", "not wired", "PRESERVE", "REPORT", "REQUEST", "separately-authorized"} {
			if !strings.Contains(string(doc), m) {
				t.Errorf("rendered routing doc must contain static conditional marker %q", m)
			}
		}
		// A command footer carries the static conditional clause on both profiles.
		cmd, err := os.ReadFile(filepath.Join(root, ".opencode", "commands", "implement.md"))
		if err != nil {
			t.Fatalf("read rendered command template: %v", err)
		}
		if !strings.Contains(string(cmd), "For git operations, follow `.opencode/docs/git-execution-routing.md`") {
			t.Errorf("command footer must keep the routing-doc pointer")
		}
		if !strings.Contains(string(cmd), "without `core/gated-commit` selected") {
			t.Errorf("command footer must carry the capability-conditional clause")
		}
		// No template syntax leaks anywhere in the rendered routing doc.
		if strings.Contains(string(doc), "{{if") || strings.Contains(string(doc), "{{ if") {
			t.Errorf("rendered routing doc must not leak template syntax")
		}
	}
}

// TestO5_DenialPayloadConditionalWording pins the Go backstop denial reason:
// the unconditional safety rule stays, and the static conditional sentence
// names the unavailable-route recovery for unselected profiles.
func TestO5_DenialPayloadConditionalWording(t *testing.T) {
	deny, reason := denyExecGitMutationPayload([]string{"git", "--no-pager", "commit"})
	if !deny {
		t.Fatal("mutation payload must deny")
	}
	for _, want := range []string{
		"commit-gate",       // sanctioned alternative (preserved pin)
		"committer agent",   // authority (preserved pin)
		"core/gated-commit", // conditional sentence: capability name
		"not wired",         // conditional sentence: honest unavailability
		"preserve the work", // conditional sentence: no-work-lost recovery
		"operator handling", // conditional sentence: authorized recovery
	} {
		if !strings.Contains(reason, want) {
			t.Errorf("denial reason must contain %q; got:\n%s", want, reason)
		}
	}
	if strings.Contains(reason, "forbidden-patterns") {
		t.Errorf("git-guard reason must not misdirect to forbidden-patterns (friction-test contract)")
	}
}

// TestO5_StaticDenialSurfaces pins the JS-layer denial wording (template
// source of truth) for the same conditional sentence, mirroring the
// forbidden-patterns-git-mutation.test.js import convention.
func TestO5_StaticDenialSurfaces(t *testing.T) {
	for _, rel := range []string{
		".opencode/repo-configs/forbidden-patterns.core.js",
		".opencode/plugins/shell-guard-core.js",
		".opencode/plugins/compaction-primitives.js",
	} {
		raw, err := corpus.CoreFS.ReadFile("templates/core/" + rel)
		if err != nil {
			t.Fatalf("read template %s: %v", rel, err)
		}
		s := string(raw)
		if strings.Contains(s, "git-mutation-bypass") || strings.Contains(s, "committer") {
			if !strings.Contains(s, "core/gated-commit") {
				t.Errorf("%s: git-routing denial/mandate text must carry the capability-conditional sentence (core/gated-commit)", rel)
			}
			if !strings.Contains(s, "preserve the work") {
				t.Errorf("%s: conditional sentence must name work preservation", rel)
			}
		}
	}
}
