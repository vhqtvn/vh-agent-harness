package cli

// Phase 3 capability-installer render tests. These exercise the resolver ->
// renderSeamStaging wiring (internal/cli/seam.go renderSeamStaging +
// internal/cli/profile.go resolveCapabilityAnswers) end-to-end through the real
// install/update/doctor verbs, then parse the rendered opencode.jsonc to assert
// which agents and which universal-agent task edges render for a given profile.
//
// Coverage:
//   - Backward-compat default: a profile with NO `capabilities:` key (the
//     current dogfood + embedded default profiles) must render ALL 21 agents
//     (9 ungated incl. the dormant plan subagent + 7 gated-commit + 5 debate)
//     byte-identically to pre-Phase-3 — the resolver's phase3BackwardCompatDefault
//     selects {core/gated-commit, core/debate} so every gated block renders.
//   - Like-for-like across render paths: install + doctor must agree (no managed
//     drift), proving the resolver wiring lives INSIDE renderSeamStaging (shared
//     by seamApply AND doctor's managed-drift re-render).
//   - Graceful degradation: a profile that selects ONLY core/debate must drop
//     the gated-commit agents AND the universal agents' task edges to them
//     (build/coordination/project-coordinator -> committer et al.; docs-steward
//     -> committer), while keeping the 9 ungated + 5 debate agents and the
//     debate task edges. Task-edge dropping is enforced by permconfig.Emit's
//     present-agent filter (internal/permconfig/emit.go), not the template
//     gates (which permconfig authoritatively overwrites — see tables.go).
//
// These are the Phase 3 acceptance tests: render parity (no behavior change for
// the current profile) and graceful degradation (the capability gate contract).

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// baselineAgents is the 8 always-on universal agent set (CoreCatalog baseline).
var baselineAgents = []string{
	"coordination",
	"build",
	"project-coordinator",
	"researcher",
	"repo-explorer",
	"planner",
	"docs-steward",
	"ship-review",
}

// ungatedAgents is every agent the template renders unconditionally: the 8
// CoreCatalog baseline agents PLUS "plan" (a dormant subagent defined directly
// in the template, disabled as a primary mode — it is NOT in CoreCatalog and
// not capability-provided, so it is never gated). It must render in every
// profile, so it participates in the render-count assertions.
var ungatedAgents = append(append([]string{}, baselineAgents...), "plan")

// orchestrators is the subset of ungated agents whose task allowlist spans the
// full gated roster (commit-message/commit-reviewer/committer AND
// debate/solution-brief). docs-steward is intentionally excluded: its task map
// carries ONLY an optional committer edge (the headline graceful-degradation
// case), so it is asserted separately from the orchestrators.
var orchestrators = []string{"build", "coordination", "project-coordinator"}

// gatedCommitAgents is the 7 agents owned by the core/gated-commit capability.
var gatedCommitAgents = []string{
	"commit-message",
	"commit-reviewer",
	"commit-reviewer-a",
	"commit-reviewer-b",
	"commit-reviewer-c",
	"commit-reviewer-d",
	"committer",
}

// debateAgents is the 5 agents owned by the core/debate capability.
var debateAgents = []string{
	"debate",
	"debate-proposer",
	"debate-critic",
	"debate-synth",
	"solution-brief",
}

// parseRenderedAgents reads the rendered opencode.jsonc and returns the set of
// agent names that rendered. It mirrors the JSON-parse idiom in
// TestSeamInstall_GateExemptAgentOmitsGateCommands.
func parseRenderedAgents(t *testing.T, root string) map[string]bool {
	t.Helper()
	cfg, err := os.ReadFile(filepath.Join(root, "opencode.jsonc"))
	if err != nil {
		t.Fatalf("read opencode.jsonc: %v", err)
	}
	var doc struct {
		Agent map[string]json.RawMessage `json:"agent"`
	}
	if err := json.Unmarshal(cfg, &doc); err != nil {
		t.Fatalf("unmarshal opencode.jsonc: %v\n--- cfg ---\n%s", err, cfg)
	}
	out := make(map[string]bool, len(doc.Agent))
	for name := range doc.Agent {
		out[name] = true
	}
	return out
}

// parseRenderedTaskEdges reads the rendered opencode.jsonc and returns, for each
// agent, the set of target agent names in its permission.task allow map.
func parseRenderedTaskEdges(t *testing.T, root string) map[string]map[string]bool {
	t.Helper()
	cfg, err := os.ReadFile(filepath.Join(root, "opencode.jsonc"))
	if err != nil {
		t.Fatalf("read opencode.jsonc: %v", err)
	}
	var doc struct {
		Agent map[string]struct {
			Permission struct {
				Task map[string]string `json:"task"`
			} `json:"permission"`
		} `json:"agent"`
	}
	if err := json.Unmarshal(cfg, &doc); err != nil {
		t.Fatalf("unmarshal opencode.jsonc: %v\n--- cfg ---\n%s", err, cfg)
	}
	out := make(map[string]map[string]bool, len(doc.Agent))
	for name, a := range doc.Agent {
		edges := make(map[string]bool)
		for target, decision := range a.Permission.Task {
			if decision == "allow" {
				edges[target] = true
			}
		}
		out[name] = edges
	}
	return out
}

// assertAgentsPresent fails the test if any name in want is absent from rendered.
func assertAgentsPresent(t *testing.T, rendered map[string]bool, want []string) {
	t.Helper()
	for _, name := range want {
		if !rendered[name] {
			t.Errorf("agent %q must render but is absent; rendered=%v", name, capRenderSortedKeys(rendered))
		}
	}
}

// assertAgentsAbsent fails the test if any name in want is present in rendered.
func assertAgentsAbsent(t *testing.T, rendered map[string]bool, want []string) {
	t.Helper()
	for _, name := range want {
		if rendered[name] {
			t.Errorf("agent %q must NOT render but is present; rendered=%v", name, capRenderSortedKeys(rendered))
		}
	}
}

func capRenderSortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	// Stable order for deterministic failure messages; no sort import needed.
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j-1] > out[j]; j-- {
			out[j-1], out[j] = out[j], out[j-1]
		}
	}
	return out
}

// TestSeamRender_BackwardCompatDefaultRendersAllCapabilities is the Phase 3
// render-PARITY proof: a profile with no `capabilities:` key (the current
// dogfood + embedded default profiles) must render all 21 agents — the resolver's
// backward-compat default selects both core seeds so every gated block renders,
// byte-identical to pre-Phase-3. If this regresses, the current profile silently
// loses agents on the next update.
func TestSeamRender_BackwardCompatDefaultRendersAllCapabilities(t *testing.T) {
	root := t.TempDir()
	seamInstallInto(t, root)
	// The installed profile is the embedded default (no `capabilities:` key),
	// so the backward-compat default applies.
	rendered := parseRenderedAgents(t, root)

	// All 21 agents must render (9 ungated + 7 gated-commit + 5 debate).
	assertAgentsPresent(t, rendered, ungatedAgents)
	assertAgentsPresent(t, rendered, gatedCommitAgents)
	assertAgentsPresent(t, rendered, debateAgents)
	if got, want := len(rendered), len(ungatedAgents)+len(gatedCommitAgents)+len(debateAgents); got != want {
		t.Errorf("agent count: got %d, want %d (rendered=%v)", got, want, capRenderSortedKeys(rendered))
	}

	// The universal agents' task edges to the gated agents must render too
	// (the per-edge gates are true under the backward-compat default). The
	// orchestrators carry the full gated roster; docs-steward carries only its
	// optional committer edge (no debate edge in CoreTaskRules).
	edges := parseRenderedTaskEdges(t, root)
	for _, orch := range orchestrators {
		task := edges[orch]
		if !task["committer"] {
			t.Errorf("%s -> committer task edge must render under backward-compat default", orch)
		}
		if !task["commit-message"] {
			t.Errorf("%s -> commit-message task edge must render under backward-compat default", orch)
		}
		if !task["debate"] {
			t.Errorf("%s -> debate task edge must render under backward-compat default", orch)
		}
	}
	if !edges["docs-steward"]["committer"] {
		t.Errorf("docs-steward -> committer task edge must render under backward-compat default")
	}
}

// TestSeamRender_DoctorLikeForLikeNoDriftOnBackwardCompat proves the resolver
// wiring lives INSIDE renderSeamStaging (not only in seamApply): the doctor
// managed-drift check re-renders via the same path, so install and doctor must
// agree byte-for-byte. A HEALTHY doctor means no gated agent block is spuriously
// flagged as drift — the exact invariant that motivated injecting the resolver
// at renderSeamStaging rather than at seamApply.
func TestSeamRender_DoctorLikeForLikeNoDriftOnBackwardCompat(t *testing.T) {
	root := t.TempDir()
	seamInstallInto(t, root)
	out := seamDoctorOut(t, root)
	if !strings.Contains(out, "result: HEALTHY") {
		t.Errorf("doctor must report HEALTHY (no managed drift) under the backward-compat default;\ngot:\n%s", out)
	}
	if strings.Contains(out, "drifted") {
		t.Errorf("doctor must report NO drifted files under the backward-compat default;\ngot:\n%s", out)
	}
}

// TestSeamRender_CapabilitySelectionGracefulDegradation proves the capability
// gate contract: a profile selecting ONLY core/debate drops the gated-commit
// agents AND the universal agents' task edges to them (graceful degradation),
// while keeping the 8 baseline + 5 debate agents and the debate task edges. This
// is the headline graceful-degradation case — docs-steward (a baseline agent)
// keeps working without its optional committer edge.
func TestSeamRender_CapabilitySelectionGracefulDegradation(t *testing.T) {
	root := t.TempDir()
	seamInstallInto(t, root)
	// Select ONLY core/debate: an explicit non-empty capabilities list disables
	// the backward-compat default, so core/gated-commit must NOT resolve.
	writeProfile(t, root, "profile: minimal\nmodules: [core]\nfeatures:\n  backlog: true\noverlays: []\npolicy_packs: []\ncapabilities:\n  - core/debate\n")
	if _, err := seamUpdateOut(t, root); err != nil {
		t.Fatalf("update with capabilities:[core/debate]: %v", err)
	}
	rendered := parseRenderedAgents(t, root)

	// Ungated (9) + debate (5) render; gated-commit (7) is absent.
	assertAgentsPresent(t, rendered, ungatedAgents)
	assertAgentsPresent(t, rendered, debateAgents)
	assertAgentsAbsent(t, rendered, gatedCommitAgents)
	if got, want := len(rendered), len(ungatedAgents)+len(debateAgents); got != want {
		t.Errorf("agent count: got %d, want %d (ungated + debate; rendered=%v)", got, want, capRenderSortedKeys(rendered))
	}

	// Graceful degradation on task edges: orchestrators drop their edges to
	// gated-commit agents but KEEP their edges to debate agents. docs-steward
	// (headline case) drops its only optional edge — committer — and is left
	// with just the "*" deny wildcard; it never had a debate edge.
	edges := parseRenderedTaskEdges(t, root)
	for _, orch := range orchestrators {
		task := edges[orch]
		if task["committer"] {
			t.Errorf("%s -> committer task edge must NOT render when core/gated-commit is unselected", orch)
		}
		if task["commit-message"] {
			t.Errorf("%s -> commit-message task edge must NOT render when core/gated-commit is unselected", orch)
		}
		if !task["debate"] {
			t.Errorf("%s -> debate task edge must render (core/debate is selected)", orch)
		}
	}
	if edges["docs-steward"]["committer"] {
		t.Errorf("docs-steward -> committer task edge must NOT render when core/gated-commit is unselected (graceful degradation)")
	}
}
