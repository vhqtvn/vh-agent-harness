package cli

// harness-dogfood project-local overlay pack — render-level tests.
//
// These mirror internal/cli/release_render_test.go for the SECOND overlay pack
// and the FIRST project-local one. They prove the pack mechanics for
// `harness-dogfood` (a dogfood pack shipping the `harness-release-readiness`
// agent): the manifest parses + grammar is valid, discoverPackContributions
// finds the project pack via OpenPackFor, selecting project/harness-dogfood
// pulls core/release (transitive hard-dep) and core/gated-commit, and opting the
// pack into the profile renders the agent at .opencode/agents/ with the
// {file:...} prompt contract pinned.
//
// HONEST scope note: the readiness REASONING (does the migration note cover the
// six changes; is the roster shrink BREAKING) lives inside the agent prompt and
// is NOT testable without a live model. These tests cover the PACK MECHANICS
// (discovery / closure / render / prompt-form) — the same surface the release
// pack's tests cover — not the agent's G1–G5 evaluation. The G6 section's
// DETERMINISTIC prose (skill-pilot evidence / S2 holds) IS pinned by
// TestHarnessDogfood_ReleaseReadinessCarriesG6Gate below: it is content a
// regression can silently delete, not model reasoning, so it is assertable.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vhqtvn/vh-agent-harness/internal/overlay"
	"github.com/vhqtvn/vh-agent-harness/internal/resolver"
)

// harnessDogfoodAgent is the agent the harness-dogfood pack provides.
const harnessDogfoodAgent = "harness-release-readiness"

// harnessDogfoodCapability is the capability id the project pack declares.
const harnessDogfoodCapability = "project/harness-dogfood"

// harnessDogfoodViaOverlaysProfile selects the pack through the expert-override
// path (the path the dogfood repo's own profile uses). Mirrors
// releaseViaOverlaysProfile.
const harnessDogfoodViaOverlaysProfile = "profile: supervised\nfeatures:\n  backlog: true\noverlays: [harness-dogfood]\npolicy_packs: []\n"

// harnessDogfoodManifestYAML mirrors the real pack's capability-manifest.yml
// exactly (id project/harness-dogfood, provides [harness-release-readiness],
// hard_deps [core/release]). Kept inline so the test is hermetic to the temp
// root and asserts the real structural contract.
const harnessDogfoodManifestYAML = "id: project/harness-dogfood\nprovides:\n  - harness-release-readiness\nhard_deps:\n  - core/release\noptional_deps: []\n"

// harnessDogfoodAppendJSONC mirrors the real pack's opencode-append.jsonc
// agent block. The {file:...} prompt form is the A-F1 contract under test.
const harnessDogfoodAppendJSONC = `{
  "agent": {
    "harness-release-readiness": {
      "description": "Harness release-readiness reporter (dogfood).",
      "mode": "subagent",
      "color": "accent",
      "prompt": "{file:.opencode/agents/harness-release-readiness.md}",
      "permission": {
        "bash": { "__placeholder__": "deny" },
        "task": { "__placeholder__": "deny" }
      }
    }
  }
}
`

// harnessDogfoodPermissionPackJSONC mirrors the real pack's permission-pack.jsonc
// (readonly + git_readonly allow, gate deny, harness deny, task {"*":"deny"},
// delegateFrom the three orchestrators, NO gateExempt — satisfies the validate
// rule: gate key present + no gateExempt).
const harnessDogfoodPermissionPackJSONC = `{
  "agents": {
    "harness-release-readiness": {
      "location": { "wildcard": "deny", "readonly": "allow", "git_readonly": "allow", "gate": "deny", "harness": "deny" },
      "task": { "*": "deny" },
      "delegateFrom": ["build", "coordination", "project-coordinator"]
    }
  }
}
`

// writeHarnessDogfoodPack materializes the project-local harness-dogfood pack
// into root's .vh-agent-harness/overlays/harness-dogfood/, mirroring the real
// repo's pack. The agent .md is minimal (content is out of test scope — the
// G1–G5 reasoning is not assertable without a live model); the manifest,
// opencode-append, and permission-pack mirror the real structural files exactly
// because those ARE the mechanics under test. Used by both the discovery/closure
// tests and the render tests.
func writeHarnessDogfoodPack(t *testing.T, root string) {
	t.Helper()
	packDir := filepath.Join(root, filepath.FromSlash(overlay.ProjectOverlaysSubdir), "harness-dogfood")
	agentsDir := filepath.Join(packDir, "agents")
	if err := os.MkdirAll(agentsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile := func(path, content string) {
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	writeFile(filepath.Join(packDir, "capability-manifest.yml"), harnessDogfoodManifestYAML)
	// Minimal agent file — the prompt-form contract only requires the file to
	// exist and be referenced as {file:...}; the G1–G5 content is not asserted.
	writeFile(filepath.Join(agentsDir, "harness-release-readiness.md"), "# harness-release-readiness (test fixture)\n")
	writeFile(filepath.Join(packDir, "opencode-append.jsonc"), harnessDogfoodAppendJSONC)
	writeFile(filepath.Join(packDir, "permission-pack.jsonc"), harnessDogfoodPermissionPackJSONC)
}

// assertHarnessDogfoodRendered is the shared assertion block for a
// harness-dogfood-selected render: the agent file lands on disk, the agent is
// registered in opencode.jsonc, the transitive closure (core/release via
// hard-dep; core/gated-commit via core/release) renders the releaser + the
// gated-commit cluster, the delegateFrom task edges inject, and the {file:...}
// prompt contract holds. Mirrors assertReleaserRendered.
func assertHarnessDogfoodRendered(t *testing.T, root string) {
	t.Helper()
	// 1. The agent file lands on disk under .opencode/agents/.
	agentPath := filepath.Join(root, ".opencode", "agents", "harness-release-readiness.md")
	if _, err := os.Stat(agentPath); err != nil {
		t.Fatalf("harness-release-readiness.md must land on disk when harness-dogfood is selected: %v", err)
	}
	body, err := os.ReadFile(agentPath)
	if err != nil {
		t.Fatalf("read harness-release-readiness.md: %v", err)
	}
	if len(body) == 0 {
		t.Errorf("harness-release-readiness.md must be non-empty")
	}

	// 2. The agent is registered in opencode.jsonc AND the hard-dep closure
	//    renders with it: core/release -> releaser, core/gated-commit -> the
	//    7-agent commit cluster.
	rendered := parseRenderedAgents(t, root)
	if !rendered[harnessDogfoodAgent] {
		t.Errorf("harness-release-readiness agent must be registered in opencode.jsonc when harness-dogfood is selected; rendered=%v", capRenderSortedKeys(rendered))
	}
	assertAgentsPresent(t, rendered, ungatedAgents)
	assertAgentsPresent(t, rendered, gatedCommitAgents)
	assertAgentsPresent(t, rendered, []string{releaserAgent, harnessDogfoodAgent})

	// 3. The delegateFrom task edges from the orchestrators must inject: each
	//    orchestrator's task map gains harness-release-readiness:allow.
	edges := parseRenderedTaskEdges(t, root)
	for _, orch := range orchestrators {
		if !edges[orch][harnessDogfoodAgent] {
			t.Errorf("%s -> harness-release-readiness task edge must render when harness-dogfood is selected (delegateFrom); %s.task=%v", orch, orch, sortedKeysOf(edges[orch]))
		}
	}

	// 4. The prompt contract (A-F1): agent.harness-release-readiness.prompt MUST
	//    be the {file:...} reference form. Pin it like assertReleaserPromptContract.
	assertHarnessDogfoodPromptContract(t, root)
}

// assertHarnessDogfoodPromptContract pins the A-F1 contract for the readiness
// agent: the rendered prompt MUST be the {file:...} reference form. Mirrors
// assertReleaserPromptContract.
func assertHarnessDogfoodPromptContract(t *testing.T, root string) {
	t.Helper()
	cfg, err := os.ReadFile(filepath.Join(root, "opencode.jsonc"))
	if err != nil {
		t.Fatalf("read opencode.jsonc: %v", err)
	}
	var doc struct {
		Agent map[string]struct {
			Prompt string `json:"prompt"`
		} `json:"agent"`
	}
	if err := json.Unmarshal(cfg, &doc); err != nil {
		t.Fatalf("unmarshal opencode.jsonc: %v\n--- cfg ---\n%s", err, cfg)
	}
	rel, ok := doc.Agent[harnessDogfoodAgent]
	if !ok {
		t.Fatalf("harness-release-readiness agent must be registered before asserting prompt contract")
	}
	if got, want := rel.Prompt, "{file:.opencode/agents/harness-release-readiness.md}"; got != want {
		t.Errorf("agent.harness-release-readiness.prompt must use the {file:...} reference form (required for prompt-file validation to stat-resolve it); got %q, want %q", got, want)
	}
}

// TestHarnessDogfood_ManifestGrammarValid asserts the project pack's manifest id
// satisfies the catalog grammar (`^[a-z][a-z0-9-]*/[a-z][a-z0-9-]*$`).
// `project/harness-dogfood` is valid; this pins it so a future rename cannot
// silently break discovery.
func TestHarnessDogfood_ManifestGrammarValid(t *testing.T) {
	m, err := resolver.ParseManifest([]byte(harnessDogfoodManifestYAML))
	if err != nil {
		t.Fatalf("parse manifest: %v", err)
	}
	if got, want := m.ID, harnessDogfoodCapability; got != want {
		t.Errorf("manifest id: got %q, want %q", got, want)
	}
	if errs := m.Validate(); len(errs) > 0 {
		t.Errorf("manifest must be structurally valid; got %d error(s): %v", len(errs), errs)
	}
}

// TestHarnessDogfood_DiscoverFindsProjectPack asserts discoverPackContributions
// finds the project-local pack via OpenPackFor (project-first discovery), tagged
// SourceProject, alongside the embedded release pack.
func TestHarnessDogfood_DiscoverFindsProjectPack(t *testing.T) {
	root := t.TempDir()
	writeHarnessDogfoodPack(t, root)

	contribs, err := discoverPackContributions(root)
	if err != nil {
		t.Fatalf("discoverPackContributions: %v", err)
	}
	var dogfoodContrib *resolver.PackContribution
	foundRelease := false
	for i := range contribs {
		if contribs[i].Pack == "harness-dogfood" {
			dogfoodContrib = &contribs[i]
		}
		if contribs[i].Manifest.ID == "core/release" {
			foundRelease = true
		}
	}
	if dogfoodContrib == nil {
		t.Fatalf("discoverPackContributions must find the project harness-dogfood pack; got %+v", contribs)
	}
	if dogfoodContrib.Source != resolver.SourceProject {
		t.Errorf("harness-dogfood contribution source: got %v, want project", dogfoodContrib.Source)
	}
	if dogfoodContrib.Manifest.ID != harnessDogfoodCapability {
		t.Errorf("harness-dogfood manifest id: got %q, want %q", dogfoodContrib.Manifest.ID, harnessDogfoodCapability)
	}
	if !foundRelease {
		t.Errorf("embedded core/release must still be discovered alongside the project pack (no shadowing by name)")
	}
}

// TestHarnessDogfood_ClosurePullsReleaseAndGatedCommit asserts selecting
// project/harness-dogfood pulls core/release (direct hard-dep) and
// core/gated-commit (transitive, via core/release's hard-dep) in the resolved
// closure. This is the transitive hard-dep closure contract for a project pack
// depending on an embedded pack.
func TestHarnessDogfood_ClosurePullsReleaseAndGatedCommit(t *testing.T) {
	root := t.TempDir()
	writeHarnessDogfoodPack(t, root)

	contribs, err := discoverPackContributions(root)
	if err != nil {
		t.Fatalf("discoverPackContributions: %v", err)
	}
	merged, err := resolver.MergeCatalogs(resolver.CoreCatalog(), contribs)
	if err != nil {
		t.Fatalf("MergeCatalogs: %v", err)
	}
	if _, ok := merged.Get(harnessDogfoodCapability); !ok {
		t.Fatalf("merged catalog must carry %s", harnessDogfoodCapability)
	}
	set, err := resolver.Resolve([]resolver.CapabilityID{harnessDogfoodCapability}, merged)
	if err != nil {
		t.Fatalf("Resolve([%s]): %v", harnessDogfoodCapability, err)
	}
	if !set.Has("core/release") {
		t.Errorf("project/harness-dogfood closure must pull core/release (direct hard_dep); got %v", set.All())
	}
	if !set.Has("core/gated-commit") {
		t.Errorf("project/harness-dogfood closure must pull core/gated-commit (transitive, via core/release); got %v", set.All())
	}
	// The closure must also surface the provided agents: releaser (core/release)
	// and harness-release-readiness (project/harness-dogfood).
	agents := set.Agents()
	if !containsStr(agents, releaserAgent) {
		t.Errorf("closure agent roster must include releaser; got %v", agents)
	}
	if !containsStr(agents, harnessDogfoodAgent) {
		t.Errorf("closure agent roster must include harness-release-readiness; got %v", agents)
	}
}

// TestSeamRender_HarnessDogfoodViaOverlaysRendersPackAndClosure proves the
// end-to-end render: opting the pack into the profile (overlays:[harness-dogfood])
// renders the agent file, registers the agent with the {file:...} prompt, pulls
// the releaser + gated-commit cluster via the hard-dep closure, injects the
// delegateFrom edges, and keeps doctor HEALTHY.
func TestSeamRender_HarnessDogfoodViaOverlaysRendersPackAndClosure(t *testing.T) {
	root := t.TempDir()
	seamInstallInto(t, root)
	writeProfile(t, root, harnessDogfoodViaOverlaysProfile)
	writeHarnessDogfoodPack(t, root)
	if _, err := seamUpdateOut(t, root); err != nil {
		t.Fatalf("update with overlays:[harness-dogfood]: %v", err)
	}
	assertHarnessDogfoodRendered(t, root)

	out := seamDoctorOut(t, root)
	if !strings.Contains(out, "result: HEALTHY") {
		t.Errorf("doctor must report HEALTHY when harness-dogfood is selected; got:\n%s", out)
	}
	if strings.Contains(out, "drifted") {
		t.Errorf("doctor must report NO drifted files when harness-dogfood is selected; got:\n%s", out)
	}
}

// containsStr reports whether s contains x. Small helper to avoid importing
// slices for a one-off membership check on a short list.
func containsStr(s []string, x string) bool {
	for _, v := range s {
		if v == x {
			return true
		}
	}
	return false
}

// TestHarnessDogfood_ReleaseReadinessCarriesG6Gate is the deterministic content
// contract for the G6 release gate (skill-pilot evidence / S2 holds). Unlike the
// G1–G5 reasoning (not assertable without a live model), the G6 section is
// deterministic prose a regression can silently delete — so this test pins its
// presence in BOTH the AUTHORITATIVE overlay source
// (.vh-agent-harness/overlays/harness-dogfood/agents/harness-release-readiness.md)
// AND its 1:1 RENDERED MIRROR (.opencode/agents/harness-release-readiness.md):
// the source catches a direct edit that drops G6; the mirror catches a stale or
// hand-edited render (the update path must regenerate it, never hand-edit it).
// If G6 drifts out of either surface, this test fails before a held-for-pilot
// skill can ship unaudited.
func TestHarnessDogfood_ReleaseReadinessCarriesG6Gate(t *testing.T) {
	root := findModuleRoot(t)
	relPaths := []string{
		filepath.Join(".vh-agent-harness", "overlays", "harness-dogfood", "agents", "harness-release-readiness.md"),
		filepath.Join(".opencode", "agents", "harness-release-readiness.md"),
	}
	for _, rel := range relPaths {
		body, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			t.Fatalf("read readiness agent %s: %v", rel, err)
		}
		assertG6GateContent(t, rel, string(body))
	}
}

// assertG6GateContent asserts the distinctive G6 tokens are present in one
// readiness agent body. Each needle is new content the pre-G6 readiness agent
// did NOT carry (or, for SATISFIED, is confirmed alongside the distinctive
// siblings), so a regression that drops the G6 section fails here.
func assertG6GateContent(t *testing.T, label, got string) {
	t.Helper()
	checks := []struct{ name, needle string }{
		{"G6 section header", "## G6 — Skill pilot evidence (S2 holds)"},
		{"G6 blocker id G6_Skill_Pilot_Evidence", "G6_Skill_Pilot_Evidence"},
		{"PENDING verdict token", "PENDING"},
		{"SATISFIED verdict token", "SATISFIED"},
		{"stable hold ID cross-check requirement", "stable hold ID"},
		{"ready:no + null-handoff behavior (scoped to G6)", "G6 blocker forces"},
	}
	for _, c := range checks {
		if !strings.Contains(got, c.needle) {
			t.Errorf("%s: missing %s — %q (G6 gate content drifted out of the readiness agent)", label, c.name, c.needle)
		}
	}
}

// TestHarnessDogfood_ReleaseReadinessCarriesG7Gate is the deterministic content
// contract for the G7 release gate (release-time DEFER enforcement, advisory
// surface). Like G6, the G7 section is deterministic prose a regression can
// silently delete — so this test pins its presence in BOTH the AUTHORITATIVE
// overlay source AND its 1:1 RENDERED MIRROR. It also re-asserts that G6 was
// NOT weakened when G7 was added (the two gates are independent and both must
// survive). If G7 drifts out of either surface, OR G6 drifts out as a side
// effect of a G7 edit, this test fails before a release can ship with an
// unaddressed DEFER.
func TestHarnessDogfood_ReleaseReadinessCarriesG7Gate(t *testing.T) {
	root := findModuleRoot(t)
	relPaths := []string{
		filepath.Join(".vh-agent-harness", "overlays", "harness-dogfood", "agents", "harness-release-readiness.md"),
		filepath.Join(".opencode", "agents", "harness-release-readiness.md"),
	}
	for _, rel := range relPaths {
		body, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			t.Fatalf("read readiness agent %s: %v", rel, err)
		}
		got := string(body)
		t.Run(rel, func(t *testing.T) {
			assertG7GateContent(t, rel, got)
			// G6 must NOT be weakened by the G7 addition — re-run the G6 contract
			// in the same body so a regression that swapped G6 for G7 fails here.
			assertG6GateContent(t, rel+" (G6-not-weakened)", got)
		})
	}
}

// assertG7GateContent asserts the distinctive G7 tokens are present in one
// readiness agent body. Each needle is content the pre-G7 readiness agent did
// NOT carry, so a regression that drops the G7 section (or weakens it to
// advisory-only without the authoritative-wrapper pairing) fails here.
func assertG7GateContent(t *testing.T, label, got string) {
	t.Helper()
	checks := []struct{ name, needle string }{
		{"G7 section header", "### G7 — release-time DEFER enforcement gate (advisory)"},
		{"G7 blocker id G7_ReleaseDeferGate", "G7_ReleaseDeferGate"},
		{"checklist header bumped to G0–G7", "G0–G7"},
		{"source:review-defer candidate selection", "source:review-defer"},
		{"source:p2-followup exclusion", "source:p2-followup"},
		{"deterministic evaluator invocation", "check-defer-triggers.js --mode=release"},
		{"wrapper authority wording (scripts/release-tag.sh)", "scripts/release-tag.sh"},
		{"advisory scope fence (G7 is ADVISORY)", "G7 itself is ADVISORY"},
		{"wrapper-authoritative restatement", "AUTHORITATIVE"},
		{"ready:no + null-handoff behavior (scoped to G7)", "G7 blocker forces"},
		{"evaluator-error blocker class", "evaluator-error class"},
		{"absent/empty tasks-dir pass policy", "Absence policy"},
		{"delegated owner entry for G7", `"for": "G7_ReleaseDeferGate"`},
		{"blockers id enum includes G7", "G6_Skill_Pilot_Evidence | G7_ReleaseDeferGate"},
		{"self-check reminder for G7", "G7 ran the deterministic release-DEFER evaluator"},
	}
	for _, c := range checks {
		if !strings.Contains(got, c.needle) {
			t.Errorf("%s: missing %s — %q (G7 gate content drifted out of the readiness agent)", label, c.name, c.needle)
		}
	}
}
