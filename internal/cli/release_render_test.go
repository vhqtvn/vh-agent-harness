package cli

// Phase-3 capability-installer overlay-integration render-level tests.
//
// These exercise the end-to-end seam (install → update → doctor) for the first
// shipped embedded overlay pack, `release`. They prove the two selection paths
// converge, the hard-dep closure pulls core/gated-commit, and the dogfood
// supervised profile keeps `release` dormant (backward-compat).
//
// Covered:
//   - Selecting `core/release` via `capabilities:` renders .opencode/agents/
//     releaser.md on disk, registers the `releaser` agent in opencode.jsonc,
//     pulls the core/gated-commit cluster (committer et al.), and injects the
//     delegateFrom task edges (build/coordination/project-coordinator → releaser).
//   - Listing `release` via `overlays:` CONVERGES to the same render as the
//     capabilities path (the two selection paths meet inside resolveCapabilityAnswers).
//   - NOT selecting release (minimal profile) renders NEITHER releaser NOR the
//     gated-commit cluster.
//   - Backward-compat: the dogfood `profile: supervised` keeps release dormant
//     (releaser absent) while keeping the supervised preset's clusters; doctor
//     HEALTHY.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vhqtvn/vh-agent-harness/internal/overlay"
	"github.com/vhqtvn/vh-agent-harness/internal/resolver"
)

// releaseViaCapabilitiesProfile selects core/release through the resolver path.
const releaseViaCapabilitiesProfile = "profile: minimal\nfeatures:\n  backlog: true\noverlays: []\npolicy_packs: []\ncapabilities:\n  - core/release\n"

// releaseViaOverlaysProfile selects release through the expert-override path.
const releaseViaOverlaysProfile = "profile: minimal\nfeatures:\n  backlog: true\noverlays: [release]\npolicy_packs: []\n"

// releaserAgent is the agent the release pack provides.
const releaserAgent = "releaser"

// assertReleaserRendered is the shared assertion block for a release-selected
// render: the agent file lands on disk, the agent is registered in opencode.jsonc,
// the gated-commit cluster (hard-dep closure) renders, and the delegateFrom
// task edges from the orchestrators inject.
func assertReleaserRendered(t *testing.T, root string) {
	t.Helper()
	// 1. The agent file lands on disk under .opencode/agents/.
	relPath := filepath.Join(root, ".opencode", "agents", "releaser.md")
	if _, err := os.Stat(relPath); err != nil {
		t.Fatalf("releaser.md must land on disk when release is selected: %v", err)
	}
	body, err := os.ReadFile(relPath)
	if err != nil {
		t.Fatalf("read releaser.md: %v", err)
	}
	if len(body) == 0 {
		t.Errorf("releaser.md must be non-empty")
	}

	// 2. The agent is registered in opencode.jsonc AND the gated-commit cluster
	//    (hard-dep closure) renders with it.
	rendered := parseRenderedAgents(t, root)
	if !rendered[releaserAgent] {
		t.Errorf("releaser agent must be registered in opencode.jsonc when release is selected; rendered=%v", capRenderSortedKeys(rendered))
	}
	// The whole baseline (9) must render (always-on), plus the gated-commit
	// cluster (7, pulled by core/gated-commit hard-dep closure), plus releaser.
	assertAgentsPresent(t, rendered, ungatedAgents)
	assertAgentsPresent(t, rendered, gatedCommitAgents)
	assertAgentsPresent(t, rendered, []string{releaserAgent})

	// 3. The delegateFrom task edges from the orchestrators must inject: each
	//    orchestrator's task map gains releaser:allow because the release
	//    permission-pack declares delegateFrom:[build,coordination,project-coordinator]
	//    and permconfig.Emit materializes those edges when the target agent is
	//    present (it is — see #2).
	edges := parseRenderedTaskEdges(t, root)
	for _, orch := range orchestrators {
		if !edges[orch][releaserAgent] {
			t.Errorf("%s -> releaser task edge must render when release is selected (delegateFrom); %s.task=%v", orch, orch, sortedKeysOf(edges[orch]))
		}
	}

	// 4. The prompt contract (A-F1): agent.releaser.prompt MUST be the
	//    {file:...} reference form — matching the 20 core agents in
	//    templates/core/opencode.jsonc.tmpl and what `overlay new` scaffolds
	//    (internal/cli/overlay_new.go). The render validator's prompt regex
	//    (internal/cli/ref_validate.go promptFileRefRe) is anchored to
	//    ^\{file:(.+)\}$, so a bare-path literal would bypass prompt-file
	//    validation entirely and could ship an agent whose safety-critical
	//    prompt fails to load under the OpenCode config contract. Pin it.
	assertReleaserPromptContract(t, root)
}

func sortedKeysOf(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j-1] > out[j]; j-- {
			out[j-1], out[j] = out[j], out[j-1]
		}
	}
	return out
}

// assertReleaserPromptContract pins the A-F1 contract: the rendered
// agent.releaser.prompt MUST be the {file:...} reference form, matching the 20
// core agents (templates/core/opencode.jsonc.tmpl) and the `overlay new`
// scaffolder (internal/cli/overlay_new.go). The render validator's prompt regex
// (internal/cli/ref_validate.go promptFileRefRe) is anchored to ^\{file:(.+)\}$,
// so a bare-path literal never matches and bypasses prompt-file validation
// entirely — a regression to the literal form is therefore a safety-critical
// contract drift this assertion catches.
func assertReleaserPromptContract(t *testing.T, root string) {
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
	rel, ok := doc.Agent[releaserAgent]
	if !ok {
		t.Fatalf("releaser agent must be registered before asserting prompt contract")
	}
	if got, want := rel.Prompt, "{file:.opencode/agents/releaser.md}"; got != want {
		t.Errorf("agent.releaser.prompt must use the {file:...} reference form (matches the 20 core agents and the overlay-new scaffolder, and is required for prompt-file validation to stat-resolve it); got %q, want %q", got, want)
	}
}

// assertReleaserAbsent is the shared assertion block for a release-NOT-selected
// render: neither releaser.md nor the releaser agent registration is present,
// and (for the minimal case) the gated-commit cluster is absent too.
func assertReleaserAbsent(t *testing.T, root string) {
	t.Helper()
	// 1. No agent file on disk.
	relPath := filepath.Join(root, ".opencode", "agents", "releaser.md")
	if _, err := os.Stat(relPath); err == nil {
		t.Errorf("releaser.md must NOT land on disk when release is unselected")
	}
	// 2. Not registered in opencode.jsonc.
	rendered := parseRenderedAgents(t, root)
	if rendered[releaserAgent] {
		t.Errorf("releaser agent must NOT be registered when release is unselected; rendered=%v", capRenderSortedKeys(rendered))
	}
}

// TestSeamRender_ReleaseViaCapabilitiesRendersPackAndClosure proves the
// capability-resolver selection path: `capabilities: [core/release]` must
// render the release pack (releaser.md + agent registration + delegateFrom
// edges) AND pull core/gated-commit via the hard-dep closure.
func TestSeamRender_ReleaseViaCapabilitiesRendersPackAndClosure(t *testing.T) {
	root := t.TempDir()
	seamInstallInto(t, root)
	writeProfile(t, root, releaseViaCapabilitiesProfile)
	if _, err := seamUpdateOut(t, root); err != nil {
		t.Fatalf("update with capabilities:[core/release]: %v", err)
	}
	assertReleaserRendered(t, root)

	// Doctor must be HEALTHY (no managed drift from the overlay render).
	out := seamDoctorOut(t, root)
	if !strings.Contains(out, "result: HEALTHY") {
		t.Errorf("doctor must report HEALTHY when release is selected via capabilities; got:\n%s", out)
	}
	if strings.Contains(out, "drifted") {
		t.Errorf("doctor must report NO drifted files when release is selected via capabilities; got:\n%s", out)
	}
}

// TestSeamRender_ReleaseViaOverlaysConvergesWithCapabilities proves the two
// selection paths converge: `overlays: [release]` (expert-override) produces
// the SAME render as `capabilities: [core/release]` (resolver). This is the
// headline Phase-3-hook convergence invariant.
func TestSeamRender_ReleaseViaOverlaysConvergesWithCapabilities(t *testing.T) {
	root := t.TempDir()
	seamInstallInto(t, root)
	writeProfile(t, root, releaseViaOverlaysProfile)
	if _, err := seamUpdateOut(t, root); err != nil {
		t.Fatalf("update with overlays:[release]: %v", err)
	}
	// The same assertion block passes for both paths — that IS the convergence.
	assertReleaserRendered(t, root)

	out := seamDoctorOut(t, root)
	if !strings.Contains(out, "result: HEALTHY") {
		t.Errorf("doctor must report HEALTHY when release is selected via overlays; got:\n%s", out)
	}
}

// TestSeamRender_ReleaseUnselectedRendersNeitherPackNorClosure proves the
// negative case: a minimal profile that does NOT select release renders
// neither the release pack nor (since minimal's preset is empty) the
// gated-commit cluster. This guards against a regression where the release
// pack leaks into every render.
func TestSeamRender_ReleaseUnselectedRendersNeitherPackNorClosure(t *testing.T) {
	root := t.TempDir()
	seamInstallInto(t, root)
	writeProfile(t, root, "profile: minimal\nfeatures:\n  backlog: true\noverlays: []\npolicy_packs: []\n")
	if _, err := seamUpdateOut(t, root); err != nil {
		t.Fatalf("update with minimal profile: %v", err)
	}
	assertReleaserAbsent(t, root)

	// Under minimal, neither release NOR gated-commit renders (minimal preset is
	// empty). The baseline 9 ungated agents are the only ones present.
	rendered := parseRenderedAgents(t, root)
	assertAgentsPresent(t, rendered, ungatedAgents)
	assertAgentsAbsent(t, rendered, gatedCommitAgents)
	if got, want := len(rendered), len(ungatedAgents); got != want {
		t.Errorf("agent count under minimal: got %d, want %d (ungated only; rendered=%v)", got, want, capRenderSortedKeys(rendered))
	}
}

// TestSeamRender_SupervisedKeepsReleaseDormant is the backward-compat proof: the
// dogfood `profile: supervised` (which selects {core/gated-commit, core/debate})
// must keep the release pack DORMANT — releaser absent, no releaser.md on disk,
// no delegateFrom edges — while the supervised clusters still render. Adding the
// release pack to the embedded tree must not break existing supervised repos.
func TestSeamRender_SupervisedKeepsReleaseDormant(t *testing.T) {
	root := t.TempDir()
	seamInstallInto(t, root)
	writeProfile(t, root, "profile: supervised\nfeatures:\n  backlog: true\noverlays: []\npolicy_packs: []\n")
	if _, err := seamUpdateOut(t, root); err != nil {
		t.Fatalf("update with profile:supervised: %v", err)
	}
	// Release is dormant: no releaser anywhere.
	assertReleaserAbsent(t, root)
	// But the supervised clusters DO render (regression guard).
	rendered := parseRenderedAgents(t, root)
	assertAgentsPresent(t, rendered, ungatedAgents)
	assertAgentsPresent(t, rendered, gatedCommitAgents)
	assertAgentsPresent(t, rendered, debateAgents)
	// No orchestrator has a releaser task edge (delegateFrom not injected).
	edges := parseRenderedTaskEdges(t, root)
	for _, orch := range orchestrators {
		if edges[orch][releaserAgent] {
			t.Errorf("%s -> releaser task edge must NOT render under supervised (release dormant)", orch)
		}
	}

	// Doctor HEALTHY (the dogfood invariant).
	out := seamDoctorOut(t, root)
	if !strings.Contains(out, "result: HEALTHY") {
		t.Errorf("doctor must report HEALTHY under supervised (dogfood); got:\n%s", out)
	}
}

// TestDiscoverPackContributions_NoManifestProjectPackShadowsEmbeddedWholly is
// the B-F1 regression for the Model X shadowing contract: a project-local
// `release` pack WITHOUT a capability-manifest.yml must shadow the embedded
// `release` pack WHOLLY — the embedded core/release capability contribution is
// DROPPED (NOT inherited), and the no-manifest project pack provides none
// (overlay-only). This keeps the resolver layer (this catalog) in agreement
// with the render layer (OpenPackFor is project-first): both see the project
// pack as "the release pack", and a no-manifest project pack is overlay-only —
// it renders its files but supplies no capability.
//
// Under the prior Model Y the embedded core/release contribution survived here
// (the project pack's missing manifest caused a `continue`, leaving the
// embedded entry as the sole contribution) while the render layer loaded the
// project pack's files via OpenPackFor — splitting pack identity between the
// two layers (B-F1).
func TestDiscoverPackContributions_NoManifestProjectPackShadowsEmbeddedWholly(t *testing.T) {
	root := t.TempDir()
	// Project `release` pack with NO capability-manifest.yml (overlay-only).
	// Its name shadows the embedded `release` pack, so the embedded pack's
	// capability contribution must be dropped entirely.
	packDir := filepath.Join(root, filepath.FromSlash(overlay.ProjectOverlaysSubdir), "release")
	if err := os.MkdirAll(filepath.Join(packDir, "agents"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Ship a unit file so the pack is a real overlay-only directory (its files
	// still render via the render loop's OpenPackFor, project-first).
	if err := os.WriteFile(filepath.Join(packDir, "agents", "releaser.md"), []byte("# project releaser\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	contribs, err := discoverPackContributions(root)
	if err != nil {
		t.Fatalf("discoverPackContributions: %v", err)
	}
	// Model X: NO release contribution survives — neither the embedded one
	// (dropped because the project pack shadows the name wholly) NOR a project
	// one (the project pack has no manifest, so it is overlay-only).
	for _, c := range contribs {
		if c.Pack == "release" {
			t.Errorf("no `release` contribution expected under Model X (embedded dropped, project is overlay-only); got %+v", c)
		}
		if c.Manifest.ID == "core/release" {
			t.Errorf("embedded core/release must be DROPPED when a project `release` pack shadows by name (Model X); got %+v", c)
		}
	}

	// Consequence at the catalog layer: core/release is ABSENT from the merged
	// catalog (embedded dropped, project provides none), so the hard-dep
	// closure cannot pull core/gated-commit through it. Selecting core/release
	// is now an unknown-id error — pinning the Model X contract.
	merged, err := resolver.MergeCatalogs(resolver.CoreCatalog(), contribs)
	if err != nil {
		t.Fatalf("MergeCatalogs: %v", err)
	}
	if _, ok := merged.Get("core/release"); ok {
		t.Errorf("merged catalog must NOT carry core/release (project pack is overlay-only, embedded dropped)")
	}
	if _, err := resolver.Resolve([]resolver.CapabilityID{"core/release"}, merged); err == nil {
		t.Errorf("Resolve([core/release]) must error on the shadowed-away capability (unknown id under Model X)")
	}
}
