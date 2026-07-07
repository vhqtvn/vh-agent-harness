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

	// 5. Outbound task edge (Part B — the permission materialization proof):
	//    the rendered releaser must carry the `releaser -> committer` allow
	//    edge (the migration-note commit delegation) AND keep `*: deny` (leaf
	//    specialist otherwise). This is the pack-only authorization the Part-A
	//    prompt contract depends on; without it the `releaser -> committer`
	//    delegation is a runtime no-op. parseRenderedTaskEdges (the helper that
	//    extracts each rendered agent's allow task map) is reused for the
	//    committer:allow half; the *:deny half needs the raw map because that
	//    helper deliberately drops deny entries.
	if !edges[releaserAgent]["committer"] {
		t.Errorf("releaser -> committer task edge must render when release is selected (the migration-note commit delegation is pack-authorized); releaser.task allow-set=%v", sortedKeysOf(edges[releaserAgent]))
	}
	assertReleaserTaskWildcardDenied(t, root)
	// 6. The gateExempt delegator render shape (B-F1 fix): the releaser's bash
	//    block must OMIT every gate-group command (the 6 commit-gate.sh mutation
	//    subcommands + uuidgen) because the pack declares gateExempt:true and
	//    omits the gate key — matching the canonical committer-delegator
	//    contract (skeleton permission-pack.jsonc + web-builder). The readonly
	//    commit-gate.sh status probe MUST stay (it is in the readonly group, not
	//    gate), proving the omission is scoped to the gate group, not the whole
	//    commit-gate.sh surface. No gateExempt field renders into opencode.jsonc
	//    — its effect is the omission itself.
	assertReleaserGateExemptShape(t, root)
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

// assertReleaserTaskWildcardDenied pins the `*: deny` half of the releaser's
// outbound task contract. parseRenderedTaskEdges (reused in
// assertReleaserRendered for the committer:allow half) captures allow edges
// only, so the deny half needs its own raw read. The releaser must stay a leaf
// except for the narrow committer delegation: `*: deny` must hold (no other
// downstream delegations). The releaser is a gateExempt committer-delegator
// (gate omitted — see assertReleaserGateExemptShape); the committer, not the
// releaser, independently holds the gate.
func assertReleaserTaskWildcardDenied(t *testing.T, root string) {
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
	rel, ok := doc.Agent[releaserAgent]
	if !ok {
		t.Fatalf("releaser agent must be registered before asserting *:deny task contract")
	}
	if got := rel.Permission.Task["*"]; got != "deny" {
		t.Errorf("releaser task map must keep *:deny (leaf specialist; narrow committer delegation only); got *=%q", got)
	}
}

// gateGroupCommands mirrors the `gate` command group from
// internal/permconfig/tables.go (CommandGroups). A gateExempt agent's rendered
// bash block must OMIT every one of these (the emitter's computeBashBlock
// `continue`s past the gate group when HasGate is false). The committed
// committer->releaser edge depends on this: a gate:deny entry on the releaser
// would bleed into the delegated committer session via OpenCode's
// deriveSubagentSessionPermission (findLast) and override the committer's
// gate:allow, breaking the gated-commit message-as-file protocol the note commit
// runs through. Asserting their ABSENCE here pins the B-F1 fix (the prior slice
// shipped `gate:deny` + no gateExempt — the forbidden middle — and
// commit-reviewer blocked it).
var gateGroupCommands = []string{
	".opencode/scripts/commit-gate.sh acquire *",
	".opencode/scripts/commit-gate.sh commit *",
	".opencode/scripts/commit-gate.sh release *",
	".opencode/scripts/commit-gate.sh heartbeat *",
	".opencode/scripts/commit-gate.sh revert *",
	".opencode/scripts/commit-gate.sh stage-message *",
	"uuidgen",
}

// assertReleaserGateExemptShape pins the gateExempt committer-delegator render
// contract: the releaser's rendered permission.bash block OMITS every gate-group
// command (the 6 commit-gate.sh mutation subcommands + uuidgen) while KEEPING
// the readonly commit-gate.sh status probe (it lives in the readonly group, so
// every agent — gate-exempt or not — gets prompt-free lock metadata reads).
//
// No `gateExempt` field renders into opencode.jsonc: gateExempt is a BUILD-TIME
// concept (internal/permconfig/emit.go resolveRules → validate →
// computeBashBlock's gate-group skip) whose observable effect is the omission of
// the gate-group entries. So this assertion checks the omission directly — the
// gate-group keys must be absent from the bash map.
//
// The companion presence check (commit-gate.sh status stays allow) proves the
// omission is scoped to the gate group, not a wholesale drop of the
// commit-gate.sh surface or a render failure.
func assertReleaserGateExemptShape(t *testing.T, root string) {
	t.Helper()
	cfg, err := os.ReadFile(filepath.Join(root, "opencode.jsonc"))
	if err != nil {
		t.Fatalf("read opencode.jsonc: %v", err)
	}
	var doc struct {
		Agent map[string]struct {
			Permission struct {
				Bash map[string]string `json:"bash"`
			} `json:"permission"`
		} `json:"agent"`
	}
	if err := json.Unmarshal(cfg, &doc); err != nil {
		t.Fatalf("unmarshal opencode.jsonc: %v\n--- cfg ---\n%s", err, cfg)
	}
	rel, ok := doc.Agent[releaserAgent]
	if !ok {
		t.Fatalf("releaser agent must be registered before asserting gateExempt shape")
	}
	bash := rel.Permission.Bash
	if bash == nil {
		t.Fatalf("releaser permission.bash block missing")
	}
	// 1. Every gate-group command must be ABSENT (the gateExempt render effect).
	for _, cmd := range gateGroupCommands {
		if _, present := bash[cmd]; present {
			t.Errorf("releaser bash block must OMIT gate-group command %q (gateExempt delegator — gate omitted so its deny cannot bleed into the delegated committer session); found %q=%q", cmd, cmd, bash[cmd])
		}
	}
	// 2. The readonly commit-gate.sh status probe MUST stay present + allow
	//    (proves the omission is scoped to the gate group, not the whole
	//    commit-gate.sh surface). It lives in the readonly group, which every
	//    agent receives regardless of gate posture.
	if got, want := bash[".opencode/scripts/commit-gate.sh status"], "allow"; got != want {
		t.Errorf("releaser bash block must keep commit-gate.sh status=allow (readonly group, present for every agent); got %q", got)
	}
	// 3. exec-ro stays (readonly) — sanity that the readonly group rendered.
	if got, want := bash["vh-agent-harness exec-ro *"], "allow"; got != want {
		t.Errorf("releaser bash block must keep vh-agent-harness exec-ro *=allow (readonly group); got %q", got)
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

// TestDiscoverPackContributions_ManifestProjectPackReplacesEmbedded is the
// symmetric Case-1 companion to
// TestDiscoverPackContributions_NoManifestProjectPackShadowsEmbeddedWholly
// (Case-2): a project-local `release` pack WITH a capability-manifest.yml
// REPLACES the embedded `release` pack's manifest WHOLLY (SourceProject wins)
// end-to-end through discovery + MergeCatalogs. The embedded core/release
// contribution is DROPPED and the project's own manifest survives.
//
// Where Case-2 (no manifest) drops the capability entirely (overlay-only pack),
// this Case-1 has the project pack SUBSTITUTE its own capability (different id +
// provides), so the merged catalog carries the project cap, not the embedded
// one. This is the end-to-end discovery-layer mirror of the resolver-layer
// TestResolveContributions_ProjectShadowsEmbeddedByPackName (which covers the
// same project-over-embedded contract at the ResolveContributions UNIT layer
// only).
func TestDiscoverPackContributions_ManifestProjectPackReplacesEmbedded(t *testing.T) {
	root := t.TempDir()
	// Project `release` pack WITH a capability-manifest.yml declaring a DIFFERENT
	// capability (project/release, provides project-releaser). The pack NAME
	// (`release`) shadows the embedded pack; the project manifest REPLACES it.
	packDir := filepath.Join(root, filepath.FromSlash(overlay.ProjectOverlaysSubdir), "release")
	if err := os.MkdirAll(filepath.Join(packDir, "agents"), 0o755); err != nil {
		t.Fatal(err)
	}
	projManifest := []byte("id: project/release\nprovides:\n  - project-releaser\nhard_deps:\n  - core/gated-commit\noptional_deps: []\n")
	if err := os.WriteFile(filepath.Join(packDir, "capability-manifest.yml"), projManifest, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(packDir, "agents", "releaser.md"), []byte("# project releaser\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	contribs, err := discoverPackContributions(root)
	if err != nil {
		t.Fatalf("discoverPackContributions: %v", err)
	}
	// Exactly ONE `release` contribution survives, and it is the PROJECT one.
	var releaseContrib *resolver.PackContribution
	releaseCount := 0
	for i := range contribs {
		if contribs[i].Pack == "release" {
			releaseCount++
			releaseContrib = &contribs[i]
		}
		if contribs[i].Manifest.ID == "core/release" {
			t.Errorf("embedded core/release must be DROPPED when a project `release` pack with a manifest shadows by name (SourceProject wins); got %+v", contribs[i])
		}
	}
	if releaseCount != 1 {
		t.Fatalf("want exactly 1 `release` contribution (project wins, embedded dropped); got %d (%v)", releaseCount, contribs)
	}
	if releaseContrib.Source != resolver.SourceProject {
		t.Errorf("surviving release contribution source: got %v, want project", releaseContrib.Source)
	}
	if releaseContrib.Manifest.ID != "project/release" {
		t.Errorf("surviving release contribution manifest id: got %q, want project/release", releaseContrib.Manifest.ID)
	}
	if len(releaseContrib.Manifest.Provides) != 1 || releaseContrib.Manifest.Provides[0] != "project-releaser" {
		t.Errorf("surviving release contribution provides: got %v, want [project-releaser]", releaseContrib.Manifest.Provides)
	}

	// Consequence at the catalog layer: core/release is ABSENT (embedded dropped
	// by project-wins shadowing), project/release is PRESENT. Selecting
	// core/release is now an unknown-id error; selecting project/release still
	// pulls core/gated-commit via the hard-dep declared in the project manifest.
	merged, err := resolver.MergeCatalogs(resolver.CoreCatalog(), contribs)
	if err != nil {
		t.Fatalf("MergeCatalogs: %v", err)
	}
	if _, ok := merged.Get("core/release"); ok {
		t.Errorf("merged catalog must NOT carry core/release (project pack replaced it)")
	}
	if _, ok := merged.Get("project/release"); !ok {
		t.Errorf("merged catalog must carry project/release (project pack's manifest wins)")
	}
	if _, err := resolver.Resolve([]resolver.CapabilityID{"core/release"}, merged); err == nil {
		t.Errorf("Resolve([core/release]) must error (embedded dropped by project-wins; unknown id)")
	}
	set, err := resolver.Resolve([]resolver.CapabilityID{"project/release"}, merged)
	if err != nil {
		t.Fatalf("Resolve([project/release]): %v", err)
	}
	if !set.Has("core/gated-commit") {
		t.Errorf("project/release closure must still pull core/gated-commit (hard_dep in project manifest); got %v", set.All())
	}
}

// TestSeamRender_ReleasePathsByteConverge strengthens the convergence invariant
// (TestSeamRender_ReleaseViaOverlaysConvergesWithCapabilities proves both paths
// satisfy the same property block) by BYTE-COMPARING the two rendered trees.
// Rendering once via `capabilities: [core/release]` and once via
// `overlays: [release]` must produce a byte-identical opencode.jsonc, the same
// set of rendered paths, and byte-identical agent files. If the two selection
// paths genuinely converge inside resolveCapabilityAnswers, no downstream render
// step can tell them apart, so the output trees must be identical.
func TestSeamRender_ReleasePathsByteConverge(t *testing.T) {
	// Path A: select release via capabilities.
	rootA := t.TempDir()
	seamInstallInto(t, rootA)
	writeProfile(t, rootA, releaseViaCapabilitiesProfile)
	if _, err := seamUpdateOut(t, rootA); err != nil {
		t.Fatalf("update path A (capabilities): %v", err)
	}
	// Path B: select release via overlays.
	rootB := t.TempDir()
	seamInstallInto(t, rootB)
	writeProfile(t, rootB, releaseViaOverlaysProfile)
	if _, err := seamUpdateOut(t, rootB); err != nil {
		t.Fatalf("update path B (overlays): %v", err)
	}

	treeA := collectRenderedTree(t, rootA)
	treeB := collectRenderedTree(t, rootB)

	// 1. Set-equality of rendered paths.
	if len(treeA.paths) != len(treeB.paths) {
		t.Errorf("rendered path count differs: capabilities=%d overlays=%d", len(treeA.paths), len(treeB.paths))
	}
	onlyA, onlyB := setDiff(treeA.paths, treeB.paths)
	if len(onlyA) != 0 {
		t.Errorf("paths rendered ONLY via capabilities (missing from overlays path): %v", onlyA)
	}
	if len(onlyB) != 0 {
		t.Errorf("paths rendered ONLY via overlays (missing from capabilities path): %v", onlyB)
	}

	// 2. releaser.md must be present in BOTH (sanity on the convergence subject).
	releaserRel := filepath.FromSlash(".opencode/agents/releaser.md")
	if _, ok := treeA.files[releaserRel]; !ok {
		t.Errorf("releaser.md must render via capabilities path")
	}
	if _, ok := treeB.files[releaserRel]; !ok {
		t.Errorf("releaser.md must render via overlays path")
	}

	// 3. opencode.jsonc must be byte-identical (the headline convergence artifact).
	if !bytesEqual(treeA.files["opencode.jsonc"], treeB.files["opencode.jsonc"]) {
		t.Errorf("opencode.jsonc must be byte-identical across the two selection paths (capabilities vs overlays)")
	}

	// 4. Every shared agent file must be byte-identical.
	agentsDir := filepath.FromSlash(".opencode/agents")
	for rel, dataA := range treeA.files {
		if !strings.HasPrefix(rel, agentsDir+string(filepath.Separator)) {
			continue
		}
		dataB, ok := treeB.files[rel]
		if !ok {
			continue // already reported via set-equality above
		}
		if !bytesEqual(dataA, dataB) {
			t.Errorf("agent file %q must be byte-identical across the two selection paths", rel)
		}
	}
}

// collectRenderedTree walks root and returns (a) the sorted set of relative
// paths and (b) a map of relative path -> file content for every file that
// landed under root after a seam install+update. Used for byte-comparing two
// independently-rendered trees.
func collectRenderedTree(t *testing.T, root string) struct {
	paths []string
	files map[string][]byte
} {
	t.Helper()
	out := struct {
		paths []string
		files map[string][]byte
	}{files: map[string][]byte{}}
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		data, rerr := os.ReadFile(path)
		if rerr != nil {
			return rerr
		}
		out.files[rel] = data
		out.paths = append(out.paths, rel)
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	sortStrings(out.paths)
	return out
}

// setDiff returns the elements in a but not b (onlyA) and in b but not a
// (onlyB). Inputs are treated as sets; duplicates are collapsed.
func setDiff(a, b []string) (onlyA, onlyB []string) {
	bs := make(map[string]bool, len(b))
	for _, x := range b {
		bs[x] = true
	}
	as := make(map[string]bool, len(a))
	for _, x := range a {
		as[x] = true
		if !bs[x] {
			onlyA = append(onlyA, x)
		}
	}
	for _, x := range b {
		if !as[x] {
			onlyB = append(onlyB, x)
		}
	}
	return onlyA, onlyB
}

// sortStrings sorts s in place (selection sort — lists here are small).
func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j-1] > s[j]; j-- {
			s[j-1], s[j] = s[j], s[j-1]
		}
	}
}

// bytesEqual is a small helper so the test does not need to import bytes solely
// for the comparison.
func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
