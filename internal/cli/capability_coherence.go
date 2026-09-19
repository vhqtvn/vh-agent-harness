package cli

// capability_coherence.go implements the O2 advisory coherence diagnostics for
// the core/gated-commit capability (task git-routing-capability-coherence).
//
// PROBLEM BEING DIAGNOSED: the gated-commit WORKFLOW guidance (routing doc,
// AGENTS core, agent prompts, command footers, denial payloads) renders
// UNCONDITIONALLY into every adopter's repo, but the committer/commit-message/
// commit-reviewer agent blocks and the `committer` task edges are emitted into
// opencode.jsonc ONLY when the profile selects core/gated-commit (preset
// `supervised` or an explicit `capabilities:` entry). On a minimal profile an
// agent that follows the unconditional guidance delegates to a committer agent
// that does not exist in the permission wiring — task deny + raw-git deny with
// denial text pointing back at the unavailable route. O5 (separate change)
// makes the guidance wording capability-conditional; THIS file adds the
// advisory diagnostics that distinguish the healthy from the broken states so
// an operator can SEE which one they have:
//
//   - coherently-unselected (minimal without gated wiring)     -> PASS, no nag
//   - selected + correctly wired (supervised)                  -> PASS
//   - selected but an agent block is missing from the config   -> WARN
//   - selected but a required caller task edge is missing      -> WARN
//   - unselected but gated wiring present (residue/hand-wired) -> INFO
//   - config unreadable / selection unresolvable               -> INFO / SKIP
//
// AUTHORITY MODEL (F3 hazard H2, adversarially reviewed): these diagnostics
// observe FILES ON DISK (doctor: the live rendered opencode.jsonc) or the
// PROSPECTIVE OUTPUT of a proposed update (install/update --dry-run: the
// staged opencode.jsonc the emitter just produced). They are ADVISORY ONLY —
// tierInfo/tierWarn, never tierFail — and they never gate a command. The
// enforcing authority at agent runtime is the opencode process's loaded
// permission table, which is refreshed only on restart; a CLI diagnostic
// cannot observe a running process. Therefore every finding that speaks about
// selection/coherence carries an UNCONDITIONAL/PROSPECTIVE restart caveat
// ("if a running opencode session predates this config, restart it to load
// the new permissions"), grounded in the routing-doc restart step
// (.opencode/docs/git-execution-routing.md → "How to update permissions"),
// never a claim about the live process.
//
// The expected wiring contract is derived from the SAME sources of truth the
// emitter uses — the resolver catalog's core/gated-commit Provides list and
// permconfig.CoreTaskRules — so the check can never drift from the emitter
// into a parallel table. Optional OVERLAY delegateFrom edges are deliberately
// NOT asserted: emit.go's present-agent filter intentionally drops task edges
// to absent agents (graceful degradation, test-pinned), so only core caller
// edges for gated-commit-owned targets are required, and only when both the
// caller block and the target block are present in the config under test.

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/vhqtvn/vh-agent-harness/internal/permconfig"
	"github.com/vhqtvn/vh-agent-harness/internal/resolver"
	"github.com/vhqtvn/vh-agent-harness/internal/substrate"
)

// gatedCommitCapID is the capability this coherence check is scoped to. The
// stranding defect is specific to core/gated-commit (the only capability whose
// workflow guidance renders unconditionally while its permission wiring is
// template-gated); media-perception and worker-read-only declare CoreOutputs
// so their guidance travels with their selection.
const gatedCommitCapID resolver.CapabilityID = "core/gated-commit"

// coherence states (closed vocabulary; each maps to exactly one doctor tier).
const (
	cohSelectedWired    = "selected-wired"      // selected; blocks + edges present
	cohUnselectedClean  = "unselected-coherent" // not selected; no gated wiring
	cohMissingBlocks    = "selected-missing-agent-blocks"
	cohMissingEdges     = "selected-missing-caller-edges"
	cohResidue          = "unselected-wiring-present"
	cohConfigUnknown    = "config-unreadable"
	cohSelectionUnknown = "selection-unknown"
)

// coherenceRestartCaveat is the H2-bound UNCONDITIONAL/PROSPECTIVE restart
// note. It is appended to any finding that speaks about selection coherence so
// a green "selected-wired" line can never be misread as proof a running
// process can commit. A CLI cannot observe processes; this caveat never claims
// to. Grounded in the routing-doc restart step, not the AGENTS.md skill-list
// cache caveat.
const coherenceRestartCaveat = "advisory file-level view — if a running opencode " +
	"session predates this config, restart opencode to load the new permissions " +
	"(.opencode/docs/git-execution-routing.md → \"How to update permissions\")"

// coherenceFinding is the pure classification result over one config document.
type coherenceFinding struct {
	State         string
	MissingBlocks []string // sorted agent names (selected; block absent)
	MissingEdges  []string // sorted "caller -> target" (selected; edge absent)
	ResidueBlocks []string // sorted agent names (unselected; block present)
}

// docAgentPresent reports whether the config has an agent block for name.
func docAgentPresent(doc map[string]any, name string) bool {
	agents, ok := doc["agent"].(map[string]any)
	if !ok {
		return false
	}
	_, ok = agents[name]
	return ok
}

// docTaskEdgeResolved reports whether caller's permission.task carries a
// resolved (allow/ask) entry for target, and whether the caller block exists
// at all. Values navigate the encoding/json any-tree (maps + strings).
func docTaskEdgeResolved(doc map[string]any, caller, target string) (resolved, callerPresent bool) {
	agents, ok := doc["agent"].(map[string]any)
	if !ok {
		return false, false
	}
	callerBlock, ok := agents[caller].(map[string]any)
	if !ok {
		return false, false
	}
	perm, ok := callerBlock["permission"].(map[string]any)
	if !ok {
		return false, true
	}
	task, ok := perm["task"].(map[string]any)
	if !ok {
		return false, true
	}
	// An absent agent entry in a rendered task map is JSON null → nil any.
	// Matching hasResolvedAgentEdge, "allow" and "ask" both count as resolved.
	dec, ok := task[target].(string)
	return ok && (dec == "allow" || dec == "ask"), true
}

// classifyGatedCommitCoherence is the pure classifier shared by the doctor
// (live) and seam (prospective) diagnostics. selected is whether the resolved
// capability selection includes core/gated-commit; provides is that
// capability's Provides agent list (from the same catalog the render used);
// doc is the parsed opencode.jsonc under test.
//
// Edge expectations mirror the emitter exactly: for each CoreTaskRules caller
// whose task list names a gated-commit-owned target, the edge is required only
// when BOTH the caller block and the target block are present — that is the
// present-agent filter's own contract, so a missing target block is reported
// once (as a block), never double-reported as every caller's missing edge.
func classifyGatedCommitCoherence(selected bool, provides []string, doc map[string]any) coherenceFinding {
	providesSet := make(map[string]bool, len(provides))
	for _, a := range provides {
		providesSet[a] = true
	}

	present := make(map[string]bool, len(provides))
	var residue []string
	for _, a := range provides {
		if docAgentPresent(doc, a) {
			present[a] = true
			if !selected {
				residue = append(residue, a)
			}
		}
	}

	if !selected {
		if len(residue) > 0 {
			sort.Strings(residue)
			return coherenceFinding{State: cohResidue, ResidueBlocks: residue}
		}
		return coherenceFinding{State: cohUnselectedClean}
	}

	var missingBlocks []string
	for _, a := range provides {
		if !present[a] {
			missingBlocks = append(missingBlocks, a)
		}
	}
	sort.Strings(missingBlocks)

	// Required caller edges: core CoreTaskRules entries targeting a
	// gated-commit-owned agent, asserted only where both endpoints render.
	callers := make([]string, 0, len(permconfig.CoreTaskRules))
	for caller := range permconfig.CoreTaskRules {
		callers = append(callers, caller)
	}
	sort.Strings(callers)
	var missingEdges []string
	for _, caller := range callers {
		for _, entry := range permconfig.CoreTaskRules[caller] {
			if !providesSet[entry.Target] || entry.Decision != permconfig.Allow {
				continue
			}
			if !present[entry.Target] {
				continue // missing target block already reported above
			}
			resolved, callerPresent := docTaskEdgeResolved(doc, caller, entry.Target)
			if callerPresent && !resolved {
				missingEdges = append(missingEdges, caller+" -> "+entry.Target)
			}
		}
	}
	sort.Strings(missingEdges)

	switch {
	case len(missingBlocks) > 0:
		return coherenceFinding{State: cohMissingBlocks, MissingBlocks: missingBlocks, MissingEdges: missingEdges}
	case len(missingEdges) > 0:
		return coherenceFinding{State: cohMissingEdges, MissingEdges: missingEdges}
	default:
		return coherenceFinding{State: cohSelectedWired}
	}
}

// gatedCommitProvides resolves the Provides list for core/gated-commit from
// the given catalog (the merged core+overlay catalog the render resolved
// against). ok=false means the capability is unknown to the catalog — the
// coherence check cannot speak about it.
func gatedCommitProvides(catalog *resolver.Catalog) ([]string, bool) {
	if catalog == nil {
		return nil, false
	}
	man, ok := catalog.Get(string(gatedCommitCapID))
	if !ok {
		return nil, false
	}
	return man.Provides, true
}

// checkCapabilityCoherence is the doctor (LIVE) coherence check. It compares
// the resolved capability selection against the LIVE rendered opencode.jsonc
// on disk. Advisory only: tierInfo/tierWarn never tierFail — a broken
// selection-to-wiring contract is surfaced for the operator to repair, not a
// health failure (the enforcing surface is the opencode permission table
// itself; see the authority comment atop this file).
func checkCapabilityCoherence(target string) checkResult {
	const name = "capability-coherence"

	// Selection: the SAME full resolution the render uses (preset ∪ explicit
	// capabilities ∪ overlay-pack manifest contributions), so an overlay that
	// pulls core/gated-commit in via its hard-dep closure is honored here too.
	_, _, catalog, selected, err := resolveCapabilityAnswers(target)
	if err != nil {
		return checkResult{name: name, tier: tierSkip,
			detail: "capability selection unresolvable (" + err.Error() + "); coherence unknown"}
	}
	provides, ok := gatedCommitProvides(catalog)
	if !ok {
		return checkResult{name: name, tier: tierSkip,
			detail: "core/gated-commit absent from the resolved catalog; nothing to check"}
	}

	raw, rerr := os.ReadFile(filepath.Join(target, opencodeJSONCRel))
	if rerr != nil {
		return checkResult{name: name, tier: tierInfo,
			detail: opencodeJSONCRel + " not readable; wiring coherence unknown (other checks own the install-state surface)"}
	}
	doc, ok := parseOpencodeConfigDoc(raw)
	if !ok {
		return checkResult{name: name, tier: tierInfo,
			detail: opencodeJSONCRel + " unparseable; wiring coherence unknown"}
	}

	f := classifyGatedCommitCoherence(selected.Has(gatedCommitCapID), provides, doc)
	switch f.State {
	case cohUnselectedClean:
		return checkResult{name: name, tier: tierPass,
			detail: "core/gated-commit not selected and no gated-commit wiring present in live " + opencodeJSONCRel +
				" (coherent inactivity — automated committing is unavailable BY SELECTION; activate via `capabilities: [core/gated-commit]` in " +
				harnessProfileName + " if agent-driven commits are wanted)"}
	case cohSelectedWired:
		return checkResult{name: name, tier: tierPass,
			detail: "core/gated-commit selected; all agent blocks and required caller task edges present in live " + opencodeJSONCRel +
				" (" + coherenceRestartCaveat + ")"}
	case cohMissingBlocks, cohMissingEdges:
		var parts []string
		if len(f.MissingBlocks) > 0 {
			parts = append(parts, "missing agent block(s): "+strings.Join(f.MissingBlocks, ", "))
		}
		if len(f.MissingEdges) > 0 {
			parts = append(parts, "missing required caller task edge(s): "+strings.Join(f.MissingEdges, ", "))
		}
		return checkResult{name: name, tier: tierWarn,
			detail: "core/gated-commit is SELECTED but the live config wiring is incomplete — " +
				strings.Join(parts, "; ") +
				". Re-run `vh-agent-harness update` to re-emit, then verify; " + coherenceRestartCaveat}
	case cohResidue:
		return checkResult{name: name, tier: tierInfo,
			detail: "core/gated-commit is NOT selected but gated-commit wiring is present in live " + opencodeJSONCRel +
				" (" + strings.Join(f.ResidueBlocks, ", ") + ") — hand-wired or prior-version residue; a harness render will not produce it."}
	default:
		return checkResult{name: name, tier: tierSkip, detail: "unhandled coherence state " + f.State}
	}
}

// warnIfGatedCommitIncoherent is the install/update PROSPECTIVE coherence
// warning. It lints the STAGED (post-emission) opencode.jsonc — the exact
// bytes this update proposes to write — against the same resolved selection
// the render used. Deliberately runs for BOTH dry-run and live applies (a
// dry-run previews the warning), mirroring warnIfDeadGrants. It NEVER converts
// emission into a hard error: install/update must remain available as repair
// paths. Silent on coherent states (no nag on healthy minimal/supervised).
func warnIfGatedCommitIncoherent(target, staging string) {
	_, _, catalog, selected, err := resolveCapabilityAnswers(target)
	if err != nil {
		return // selection unresolvable: render paths surface resolution errors
	}
	provides, ok := gatedCommitProvides(catalog)
	if !ok {
		return
	}
	raw, rerr := os.ReadFile(filepath.Join(staging, opencodeJSONCRel))
	if rerr != nil {
		return // emission failures surface through the render path itself
	}
	doc, ok := parseOpencodeConfigDoc(raw)
	if !ok {
		return
	}

	f := classifyGatedCommitCoherence(selected.Has(gatedCommitCapID), provides, doc)
	switch f.State {
	case cohMissingBlocks, cohMissingEdges:
		var lines []string
		for _, b := range f.MissingBlocks {
			lines = append(lines, "- agent block missing: "+b)
		}
		for _, e := range f.MissingEdges {
			lines = append(lines, "- required caller task edge missing: "+e)
		}
		fmt.Fprintf(os.Stderr, `
vh-agent-harness WARNING: gated-commit capability/wiring mismatch in the PROSPECTIVE output of this update.
  core/gated-commit is selected in %s but the emitted %s omits:
    %s
  The emitted permission table would deny delegation to the missing agent(s),
  reproducing the stranding defect this diagnostic exists to catch. This is
  advisory: update/install are NOT blocked. Re-run with a rebuilt binary if the
  emitter is suspected; %s.

`, harnessProfileName, opencodeJSONCRel, strings.Join(lines, "\n    "), coherenceRestartCaveat)
	case cohResidue:
		fmt.Fprintf(os.Stderr, `
vh-agent-harness WARNING: gated-commit wiring present in the PROSPECTIVE output without selection.
  core/gated-commit is NOT selected in %s but the emitted %s still contains:
    - %s
  A harness render does not produce gated wiring under an unselected profile;
  this indicates an emitter/template regression or unexpected transform input.
  Advisory only; update/install are NOT blocked.

`, harnessProfileName, opencodeJSONCRel, strings.Join(f.ResidueBlocks, "\n    - "))
	}
}

// routingDocLiveRel is the live repo-relative path of the canonical routing
// doc — the entry point every other guidance surface points at for the
// capability-conditional workflow (O5).
const routingDocLiveRel = ".opencode/docs/git-execution-routing.md"

// warnIfRoutingWordingPreserved implements the F3 H1 resolution's update-note:
// when this update PRESERVES the adopter's locally-edited routing doc
// (ActionManagedDiverged — the origin-hash three-way skip; never a clobber),
// the operator's copy keeps the prior wording. Because the harness's routing
// guidance is capability-conditional in this release, a preserved copy can
// contradict the rendered permission wiring — surface the divergence so the
// operator re-adopts the new wording deliberately. Runs on the ApplyReport
// (plan outcomes on dry-run, executed outcomes live), advisory stderr only.
//
// The composed root AGENTS.md in core/mission-split repos is a DIFFERENT,
// deliberate exception: it is force-regenerated by composeAgentsMd after apply
// (seam.go) and its divergence is WARNed by checkAgentsComposition. This note
// is about the preserved platform_managed routing surfaces only.
func warnIfRoutingWordingPreserved(report *substrate.ApplyReport) {
	if report == nil {
		return
	}
	for _, o := range report.Outcomes {
		if o.Path == routingDocLiveRel && o.Action == substrate.ActionManagedDiverged {
			fmt.Fprintf(os.Stderr, `
vh-agent-harness NOTE: %s was PRESERVED with your local edits; this update did not overwrite it.
  The harness git-routing guidance changed in this release to capability-
  conditional wording (see the staged copy's "Capability condition" section):
  the committer route is described as available only where core/gated-commit
  is selected. Your preserved copy still carries the prior unconditional
  wording, which may direct agents to a route your rendered permission wiring
  denies. Re-adopt deliberately: diff your copy against the staged wording and
  port your edits onto the new conditional model (advisory only).

`, routingDocLiveRel)
			return
		}
	}
}
