package permconfig

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/vhqtvn/vh-agent-harness/internal/jsonc"
)

// Emit is the canonical permission-pipeline entry point. It takes a rendered
// opencode.jsonc (template conditionals already expanded by the text renderer),
// overwrites every permission.bash and permission.task block from the
// Go-canonical tables merged with overlay packs, adds feature-gated entries,
// and returns deterministic normalized JSONC (Q1b: sorted top-level keys,
// controlled-order permission blocks via orderedMap, 4-space indent, comments
// dropped, trailing newline).
//
// Emit is the no-transform path: extra is nil, so output is byte-identical to
// pre-transform behavior. Use EmitWithExtra to merge permission-transform
// (config-transform.mjs) output.
//
// Task-edge capability gating: each agent's task allowlist is filtered to the
// agents actually present in the rendered config. When a template capability
// gate (e.g. {{if .capabilities.gated_commit}}) hides an agent block, every
// task edge pointing at that agent is dropped here so no orchestrator carries
// a dangling allow entry. This is the enforcement seam for Phase 3 capability
// gating; the template's own task-edge gates are a self-describing scaffold
// (consistent with the scaffold-overwrite philosophy documented in tables.go)
// that this emitter authoritatively supersedes.
//
// The caller is renderSeamStaging (internal/cli/seam.go), invoked after the
// core render + overlay merge + permission-pack materialization. Both
// seamApply (install/update) and doctor (drift check) call the same pipeline,
// so doctor auto-coheres with the emitted canonical form.
func Emit(input []byte, packs []Pack, features Features) ([]byte, error) {
	return emitCore(input, packs, features, nil)
}

// EmitWithExtra is the transform-aware entry point. It behaves exactly like Emit
// but additionally merges extra bash entries (from the validated permission-
// transform output) into the named agents' bash blocks. Each agent in extra must
// be present in the rendered config (fail-closed otherwise). The extra entries
// are validated for non-empty patterns, valid decisions, no duplicates, and no
// protected-key collisions before emission.
func EmitWithExtra(input []byte, packs []Pack, features Features, extra map[string][]BashEntry) ([]byte, error) {
	return emitCore(input, packs, features, extra)
}

// emitCore is the shared pipeline for Emit (no transform) and EmitWithExtra
// (transform active). When extra is nil or empty, output is byte-identical to
// the pre-transform Emit — this is the regression guarantee (test matrix #1).
func emitCore(input []byte, packs []Pack, features Features, extra map[string][]BashEntry) ([]byte, error) {
	root, err := jsonc.Parse(input)
	if err != nil {
		return nil, fmt.Errorf("permconfig: parse opencode.jsonc: %w", err)
	}

	// Resolve the active rule set: deep-copy core tables, merge overlay packs
	// (overlay agent location/task rules, gateExempt, delegateFrom edges).
	locations, tasks, gateExempt := resolveRules(packs)

	// Compute the set of agents actually present in the rendered config. The
	// transform may only target rendered agents; targeting a non-rendered agent
	// (capability-gated out, or unknown) is a hard error.
	agents, _ := root["agent"].(map[string]any)
	present := make(map[string]bool, len(agents))
	for name := range agents {
		present[name] = true
	}

	if err := mergeExtraBash(locations, present, extra); err != nil {
		return nil, fmt.Errorf("permconfig: %w", err)
	}

	if err := validate(locations, tasks, gateExempt); err != nil {
		return nil, fmt.Errorf("permconfig: %w", err)
	}

	// Overwrite the top-level permission.bash block (the "default" location).
	// This is the only place features.backlog adds an entry.
	if perm, ok := root["permission"].(map[string]any); ok {
		if _, ok := perm["bash"]; ok {
			perm["bash"] = computeBashBlock(locations["default"], "default", features)
		}
		// Overwrite the top-level edit decision from the canonical tables.
		// The "default" location carries flat edit (Ask) and no overrides, so
		// this is a no-op byte-wise versus the template scaffold — but owning
		// it here means doctor compares edit for the top-level block too.
		if _, ok := perm["edit"]; ok {
			perm["edit"] = computeEditBlock(locations["default"])
		}
	}

	// present was already computed above for the transform-merge check. It is
	// reused below for the task-edge capability-gating filter.

	// Overwrite each agent's permission.bash and permission.task blocks.
	// Iteration order over the Go map is irrelevant — json.MarshalIndent sorts
	// agent keys alphabetically on output, so the result is deterministic.
	for name, rule := range locations {
		if name == "default" {
			continue
		}
		agentBlock, ok := agents[name].(map[string]any)
		if !ok {
			continue // defensive: location has no agent block in config
		}
		perm, ok := agentBlock["permission"].(map[string]any)
		if !ok {
			continue
		}
		if _, ok := perm["bash"]; ok {
			// Backlog entry is top-level only; agents never get it.
			perm["bash"] = computeBashBlock(rule, name, features)
		}
		if _, ok := perm["task"]; ok {
			perm["task"] = computeTaskBlock(tasks[name], present)
		}
		// Overwrite the agent's edit decision from the canonical tables. Every
		// agent emits flat edit EXCEPT the committer, whose EditOverrides
		// produces the object form { "*": "deny", "tmp/...": "allow" }.
		if _, ok := perm["edit"]; ok {
			perm["edit"] = computeEditBlock(rule)
		}
	}

	out, err := json.MarshalIndent(root, "", "    ")
	if err != nil {
		return nil, fmt.Errorf("permconfig: marshal: %w", err)
	}
	return append(out, '\n'), nil
}

// mergeExtraBash folds validated transform-contributed extra bash entries into
// the resolved location rules. Each agent named in extra must be present in the
// rendered config (present[agent]==true); targeting a non-rendered agent is a
// hard error (fail-closed). Structural validation of the entries themselves
// (non-empty pattern, valid decision, no duplicates, no protected-key
// collisions) happens in validate, which runs after this merge.
func mergeExtraBash(locations map[string]LocationRule, present map[string]bool, extra map[string][]BashEntry) error {
	for agentName, entries := range extra {
		if len(entries) == 0 {
			continue
		}
		if !present[agentName] {
			return fmt.Errorf("transform targets agent %q which is not in the rendered roster", agentName)
		}
		loc, ok := locations[agentName]
		if !ok {
			return fmt.Errorf("transform targets agent %q which has no location rule", agentName)
		}
		loc.ExtraBash = entries
		locations[agentName] = loc
	}
	return nil
}

// resolveRules deep-copies the core tables and merges overlay pack
// contributions: overlay agent location/task rules replace core entries (or
// add new agents), gateExempt flags extend the base set, and delegateFrom
// edges inject "{agentName}: allow" into each declaring orchestrator's task
// allowlist (appended after core entries, matching the resolver's insertion
// semantics).
func resolveRules(packs []Pack) (locations map[string]LocationRule, tasks map[string][]TaskEntry, gateExempt map[string]bool) {
	locations = make(map[string]LocationRule, len(CoreLocationRules))
	for k, v := range CoreLocationRules {
		// Core tables declare HarnessPolicy directly (the preferred channel).
		// normalizeHarnessFields derives the scalar Harness/DevSh fields from
		// the policy so emission and validation read consistent values. A core
		// table that fails normalization is a compile-time authoring bug —
		// panic rather than silently emit an invalid config. (Overlay-pack
		// rules arriving via parseLocation are normalized defensively inside
		// validate below.)
		if err := normalizeHarnessFields(&v); err != nil {
			panic(fmt.Sprintf("permconfig: core location rule %q fails normalizeHarnessFields: %v", k, err))
		}
		locations[k] = v // LocationRule is a value type; no deep copy needed
	}

	tasks = make(map[string][]TaskEntry, len(CoreTaskRules))
	for k, v := range CoreTaskRules {
		cp := make([]TaskEntry, len(v))
		copy(cp, v)
		tasks[k] = cp
	}

	gateExempt = make(map[string]bool, len(GateExemptBase))
	for k, v := range GateExemptBase {
		gateExempt[k] = v
	}

	for _, pack := range packs {
		// Iterate pack agents in SORTED key order so delegateFrom edge
		// injection is byte-stable (Go map iteration is randomized; two
		// agents sharing a delegateFrom target would otherwise append in
		// nondeterministic order, breaking the deterministic-output contract).
		agentNames := make([]string, 0, len(pack.Agents))
		for name := range pack.Agents {
			agentNames = append(agentNames, name)
		}
		sort.Strings(agentNames)
		for _, name := range agentNames {
			agent := pack.Agents[name]
			locations[name] = agent.Location
			if len(agent.Task) > 0 {
				tasks[name] = agent.Task
			} else {
				delete(tasks, name) // overlay agent with no task → no task block
			}
			if agent.GateExempt {
				gateExempt[name] = true
			}
			// Inject delegateFrom edges: each declaring orchestrator gains
			// "{name}: allow" appended to its task allowlist.
			for _, orch := range agent.DelegateFrom {
				tasks[orch] = append(tasks[orch], TaskEntry{Target: name, Decision: Allow})
			}
		}
	}

	// Normalize all resolved rules so downstream consumers (computeBashBlock,
	// validate) read consistent HarnessPolicy/Harness/DevSh values. Core-table
	// rules are already normalized (idempotent no-op); pack-sourced rules may
	// arrive with only a scalar channel (Harness or DevSh) set. Normalization
	// errors here are not fatal — validate re-normalizes defensively and
	// surfaces them as hard errors. But successful normalization populates
	// HarnessPolicy from the scalar so computeBashBlock reads the correct
	// policy.
	for name, rule := range locations {
		_ = normalizeHarnessFields(&rule)
		locations[name] = rule
	}

	return locations, tasks, gateExempt
}

// computeBashBlock renders the permission.bash orderedMap for one location.
// The canonical order (matching the legacy resolver's renderBashLines):
//
//  1. "*" (wildcard decision)
//  2. All command-group entries (readonly + git_readonly + gate, skipping gate
//     for gate-exempt agents), each command paired with its group's decision,
//     ALL sorted by length-ascending then byte-locale. The backlog entry (if
//     enabled and this is the top-level "default" location) participates in
//     this same sort. For read_only agents, canonical read-only harness verbs
//     (HarnessReadOnlyCommandsSet members) are SKIPPED here — they are emitted
//     as specific allows in region 4b instead, and emitting them here would
//     produce a dead entry (shadowed by the 4a deny) plus a duplicate key.
//  3. Transform-contributed extra entries (rule.ExtraBash), sorted
//     length-then-locale. These are merged AFTER the command-group region so
//     that an extra allow (e.g. "./dev.sh *") wins over the leading "*": "deny"
//     under OpenCode's findLast (last-match-wins) semantics. Protected-key
//     collisions with "*", command-group commands, "vh-agent-harness *", or any
//     canonical read-only harness verb are rejected at validate time.
//  4. The policy-owned harness region:
//     - 4a: the broad "vh-agent-harness *" entry carrying the policy's
//     wildcardDecision (allow/ask/deny scalar, or deny for read_only).
//     - 4b: for read_only ONLY, each canonical safe read-only harness verb
//     (HarnessReadOnlyCommands) emitted as "allow" AFTER 4a. Under findLast
//     these later specific allows win over the earlier broad deny, so
//     read-only specialists get prompt-free access to safe inspection verbs
//     while every mutation/shell/diagnostics-export/unknown verb stays
//     denied. allow/ask/deny agents emit NO 4b region.
func computeBashBlock(rule LocationRule, locationName string, features Features) *orderedMap {
	om := newOrderedMap()
	om.set("*", string(rule.Wildcard))

	type cmdEntry struct{ cmd, decision string }
	var entries []cmdEntry

	for _, group := range CommandGroups {
		var decision Decision
		switch group.Name {
		case "readonly":
			decision = rule.Readonly
		case "git_readonly":
			decision = rule.GitReadonly
		case "gate":
			if !rule.HasGate {
				continue // gate-exempt: omit the gate key entirely
			}
			decision = rule.Gate
		default:
			continue
		}
		for _, cmd := range group.Commands {
			// read_only agents emit the canonical read-only harness verbs as
			// specific allows in region 4b AFTER the 4a deny. If a command-
			// group entry is also a canonical read-only command (today only
			// "vh-agent-harness exec-ro *"), emitting it here would produce a
			// dead entry (shadowed by the later 4a deny) AND a duplicate of
			// the 4b allow. Skip it for read_only agents; non-read_only agents
			// keep the legacy emission (the entry is harmlessly shadowed by
			// their scalar "vh-agent-harness *" entry but stays for byte-
			// stability with the pre-read_only output).
			if rule.HarnessPolicy == HarnessPolicyReadOnly && HarnessReadOnlyCommandsSet[cmd] {
				continue
			}
			entries = append(entries, cmdEntry{cmd, string(decision)})
		}
	}

	// Feature-gated backlog entry — top-level only, participates in the sort.
	if features.Backlog && locationName == "default" {
		entries = append(entries, cmdEntry{BacklogCommand, string(Allow)})
	}

	// Sort by length ascending, then byte comparison (== localeCompare for ASCII).
	sort.Slice(entries, func(i, j int) bool {
		if len(entries[i].cmd) != len(entries[j].cmd) {
			return len(entries[i].cmd) < len(entries[j].cmd)
		}
		return entries[i].cmd < entries[j].cmd
	})

	for _, e := range entries {
		om.set(e.cmd, e.decision)
	}

	// Merge transform-contributed extra entries (region 3): AFTER the
	// command-group sort, BEFORE the devSh entry. Same length-then-locale sort
	// for determinism. Protected-key and duplicate validation is in validate.
	if len(rule.ExtraBash) > 0 {
		extra := make([]cmdEntry, 0, len(rule.ExtraBash))
		for _, e := range rule.ExtraBash {
			extra = append(extra, cmdEntry{e.Pattern, string(e.Decision)})
		}
		sort.Slice(extra, func(i, j int) bool {
			if len(extra[i].cmd) != len(extra[j].cmd) {
				return len(extra[i].cmd) < len(extra[j].cmd)
			}
			return extra[i].cmd < extra[j].cmd
		})
		for _, e := range extra {
			om.set(e.cmd, e.decision)
		}
	}

	// Region 4 — the policy-owned harness region. Emission depends on the
	// agent's HarnessPolicy:
	//   - read_only: emit "vh-agent-harness *": "deny" FIRST (region 4a, the
	//     catch-all that denies every mutation/shell/diagnostics/unknown verb),
	//     THEN emit each canonical safe read-only verb as "allow" AFTER (region
	//     4b). Under findLast the later specific allows win over the earlier
	//     broad deny — this is the ONLY correct shape under findLast.
	//   - allow/ask/deny: emit only the scalar "vh-agent-harness *":
	//     <wildcardDecision> (region 4a, no 4b exceptions). Transform-injected
	//     harness patterns in region 3 are inert under findLast (this 4a scalar
	//     is later and wins).
	om.set(DevShCommand, string(rule.HarnessPolicy.wildcardDecision()))
	if rule.HarnessPolicy == HarnessPolicyReadOnly {
		for _, cmd := range HarnessReadOnlyCommands {
			om.set(cmd, string(Allow))
		}
	}
	return om
}

// computeEditBlock renders the permission.edit value for one location.
//
// Flat case: when the agent has a BROAD Edit=Allow AND no EditOverrides, edit
// is a single decision string "allow" (build, docs-steward). tmp/** is already
// covered by their broad allow, so no object form is needed.
//
// Object case (every other agent): edit is an OBJECT map {"<pattern>":
// "<action>"} consumed by OpenCode with findLast semantics
// (permission/evaluate.ts). The "*" entry (carrying the Edit decision — Deny
// for read-only agents, Ask for the top-level default) is emitted FIRST, then
// any EditOverrides (the committer's scoped commit-gate message glob), then
// the UNIVERSAL disposable-scratch carve-out TmpWriteGlob="tmp/**": allow LAST.
// findLast picks the LAST matching pattern, so the broad decision must precede
// every narrow allow. This lets every agent Write the gitignored, watcher-
// ignored `tmp/**` scratch surface (the sanctioned disposable area) while every
// OTHER edit decision stays exactly as it was — read-only agents still cannot
// touch the source tree, the committer still cannot Write outside its message
// glob + tmp, and overlay-pack agents lacking an edit key (e.g. the releaser)
// inherit the top-level default's tmp carve-out.
func computeEditBlock(rule LocationRule) any {
	if rule.Edit == Allow && len(rule.EditOverrides) == 0 {
		return string(rule.Edit)
	}
	om := newOrderedMap()
	// Broad decision FIRST (Deny for read-only agents, Ask for the top-level
	// default). findLast picks the LAST matching rule, so the broad decision
	// must precede every narrow allow.
	om.set("*", string(rule.Edit))
	// EditOverrides (the committer's scoped commit-gate message glob) in
	// declared order, before the universal tmp carve-out so the more specific
	// glob stays adjacent to its rationale.
	for _, o := range rule.EditOverrides {
		om.set(o.Pattern, string(o.Decision))
	}
	// UNIVERSAL disposable-scratch carve-out LAST. tmp/ is gitignored
	// (.gitignore) and watcher-ignored, so every agent may Write there without
	// a prompt and without broadening any other edit decision. Emitted LAST so
	// findLast resolves tmp paths to allow even when a later EditOverride
	// existed (the committer carries both its scoped glob AND this one).
	om.set(TmpWriteGlob, string(Allow))
	return om
}

// computeTaskBlock renders the permission.task orderedMap in insertion order
// (the resolver's Object.entries order). The core tables encode this order
// explicitly via []TaskEntry slices; overlay pack tasks are parsed with "*"
// first (natural alphabetical sort puts ASCII 42 before lowercase letters).
//
// Edges to agents absent from the rendered config (present[target]==false) are
// skipped — this is the capability-gating seam. When a capability gate hides an
// agent block in the template, every task edge pointing at that agent is
// dropped here so no orchestrator carries a dangling allow entry. The "*"
// wildcard is always preserved regardless of presence.
func computeTaskBlock(rule []TaskEntry, present map[string]bool) *orderedMap {
	om := newOrderedMap()
	for _, e := range rule {
		if e.Target != "*" && !present[e.Target] {
			continue
		}
		om.set(e.Target, string(e.Decision))
	}
	return om
}

// protectedBashKeys returns the set of bash-block keys that the transform's
// extra entries may NEVER collide with: the "*" wildcard, every command-group
// command (readonly + git_readonly + gate), the backlog command, the fixed
// "vh-agent-harness *" devSh entry, and every canonical read-only harness verb
// (HarnessReadOnlyCommands). Colliding with any of these would either shadow a
// canonical entry, corrupt the "vh-agent-harness *"-last boundary, or let a
// transform weaken the read_only deny-first contract by injecting a hostile
// decision ahead of the canonical 4b allows.
func protectedBashKeys() map[string]bool {
	protected := map[string]bool{"*": true, DevShCommand: true, BacklogCommand: true}
	for _, group := range CommandGroups {
		for _, cmd := range group.Commands {
			protected[cmd] = true
		}
	}
	for _, cmd := range HarnessReadOnlyCommands {
		protected[cmd] = true
	}
	return protected
}

// validateExtraBash checks transform-contributed extra bash entries for one
// agent: each pattern must be non-empty, each decision a valid enum value, no
// duplicate patterns within the agent's set, and no collision with a protected
// key (the wildcard, command-group commands, the backlog command, or the
// devSh entry). All checks are fail-closed.
func validateExtraBash(agentName string, entries []BashEntry) error {
	if len(entries) == 0 {
		return nil
	}
	protected := protectedBashKeys()
	seen := make(map[string]bool, len(entries))
	for _, e := range entries {
		if e.Pattern == "" {
			return fmt.Errorf("agent %q: extra bash entry has empty pattern", agentName)
		}
		if !validDecision(e.Decision) {
			return fmt.Errorf("agent %q: extra bash entry %q decision %q invalid", agentName, e.Pattern, e.Decision)
		}
		if seen[e.Pattern] {
			return fmt.Errorf("agent %q: duplicate extra bash pattern %q", agentName, e.Pattern)
		}
		if protected[e.Pattern] {
			return fmt.Errorf("agent %q: extra bash pattern %q collides with a protected key (wildcard, command group, backlog, or vh-agent-harness entry)", agentName, e.Pattern)
		}
		seen[e.Pattern] = true
	}
	return nil
}

// validate ports the legacy resolver's validateRules() contract:
//
//   - Every location rule has valid wildcard/devSh/readonly/git_readonly decisions.
//   - Gate-exempt agents must NOT carry a gate key; all others MUST.
//   - Every agent location (non-"default") has a task rule.
//   - Every task rule references known agent locations, starts with "*", and
//     has valid decisions.
//   - No duplicate commands across groups + wildcard + devSh.
//
// This runs at build time (inside Emit), so a broken overlay pack fails the
// update/doctor rather than silently producing an invalid config.
func validate(locations map[string]LocationRule, tasks map[string][]TaskEntry, gateExempt map[string]bool) error {
	// Location rule decisions.
	for name, rule := range locations {
		// normalizeHarnessFields defensively reconciles the three harness
		// channels for rules arriving from any source (core tables are already
		// normalized in resolveRules; overlay packs arrive via parseLocation;
		// test literals may construct rules directly). A conflict is a hard
		// error. `rule` is a value-typed iteration copy; normalizing it does
		// not mutate the map, but the subsequent validDecision checks read the
		// normalized scalar fields, which is what matters here.
		if err := normalizeHarnessFields(&rule); err != nil {
			return fmt.Errorf("agent %q: %w", name, err)
		}
		if !validDecision(rule.Wildcard) {
			return fmt.Errorf("agent %q: wildcard decision %q invalid", name, rule.Wildcard)
		}
		if !validDecision(rule.Readonly) {
			return fmt.Errorf("agent %q: readonly decision %q invalid", name, rule.Readonly)
		}
		if !validDecision(rule.GitReadonly) {
			return fmt.Errorf("agent %q: git_readonly decision %q invalid", name, rule.GitReadonly)
		}
		if !validDecision(rule.DevSh) {
			return fmt.Errorf("agent %q: devSh decision %q invalid", name, rule.DevSh)
		}
		if !validDecision(rule.Edit) {
			return fmt.Errorf("agent %q: edit decision %q invalid", name, rule.Edit)
		}
		if gateExempt[name] {
			if rule.HasGate {
				return fmt.Errorf("agent %q: gate-exempt but rule has gate key — must be omitted (OpenCode deriveSubagentSessionPermission would bleed parent gate deny into committer)", name)
			}
		} else {
			if !rule.HasGate {
				return fmt.Errorf("agent %q: not gate-exempt but rule lacks gate key", name)
			}
			if !validDecision(rule.Gate) {
				return fmt.Errorf("agent %q: gate decision %q invalid", name, rule.Gate)
			}
		}
		// Transform-contributed ExtraBash validation.
		if err := validateExtraBash(name, rule.ExtraBash); err != nil {
			return err
		}
	}

	// Every agent location (non-"default") must have a task rule.
	for name := range locations {
		if name == "default" {
			continue
		}
		if _, ok := tasks[name]; !ok {
			return fmt.Errorf("agent %q: has location rule but no task rule", name)
		}
	}

	// Task rule targets and decisions.
	for name, rule := range tasks {
		if _, ok := locations[name]; !ok {
			return fmt.Errorf("task rule for %q: agent not in location rules", name)
		}
		if name == "default" {
			continue // "default" has bash only; no task block expected
		}
		if len(rule) == 0 || rule[0].Target != "*" {
			return fmt.Errorf("agent %q: task rule must start with *", name)
		}
		for _, e := range rule {
			// Validate the decision for ALL entries (including "*") before
			// skipping the target-existence check for the wildcard.
			if !validDecision(e.Decision) {
				return fmt.Errorf("agent %q: task target %q decision %q invalid", name, e.Target, e.Decision)
			}
			if e.Target == "*" {
				continue
			}
			if _, ok := locations[e.Target]; !ok {
				return fmt.Errorf("agent %q: task target %q is not a known agent location", name, e.Target)
			}
		}
	}

	// No duplicate commands.
	seen := map[string]bool{}
	for _, group := range CommandGroups {
		for _, cmd := range group.Commands {
			if seen[cmd] {
				return fmt.Errorf("duplicate command %q across command groups", cmd)
			}
			seen[cmd] = true
		}
	}
	return nil
}

// GenerateAllowedCommandsJS produces the allowed-commands.js content from the
// Go-canonical CommandGroups tables. The output matches the exact export shape
// that shell-guard-core.js imports at runtime:
//
//	export const GIT_MUTATION_VERBS = [ ...21 verbs... ];
//	export const COMMANDS = {
//	    readonly: [...],
//	    git_readonly: [...],
//	    gate: [...],
//	};
//
// GIT_MUTATION_VERBS is emitted from the Go-canonical GitMutationVerbs slice
// (tables.go) so shell-guard and the exec-ro classifier share ONE verb set with
// no drift — shell-guard imports this array (not a hardcoded JS copy) to build
// its `git-mutation-bypass` regex. COMMANDS is emitted LAST so the file still
// ends with the `};\n` that the shape test (TestGenerateAllowedCommandsJS_Shape)
// asserts; GIT_MUTATION_VERBS is emitted FIRST.
//
// The file is platform_managed and regenerated on every update; it is a
// compatibility artifact (shell-guard requires the JS module export). The Go
// tables are the authority (Q2c).
func GenerateAllowedCommandsJS() []byte {
	var buf bytes.Buffer
	buf.WriteString("// GENERATED by `vh-agent-harness update` from internal/permconfig/tables.go.\n")
	buf.WriteString("// DO NOT EDIT — changes are overwritten on the next update.\n")
	buf.WriteString("// To add project-specific deny-rules, use .opencode/repo-configs/forbidden-patterns.project.js.\n")
	// GIT_MUTATION_VERBS — single canonical source shared by shell-guard (JS) and
	// the exec-ro classifier (Go). Emitted first so COMMANDS stays last.
	buf.WriteString("export const GIT_MUTATION_VERBS = [\n")
	for _, verb := range GitMutationVerbs {
		fmt.Fprintf(&buf, "    %q,\n", verb)
	}
	buf.WriteString("];\n")
	buf.WriteString("export const COMMANDS = {\n")
	for _, group := range CommandGroups {
		fmt.Fprintf(&buf, "    %s: [\n", group.Name)
		for _, cmd := range group.Commands {
			fmt.Fprintf(&buf, "        %q,\n", cmd)
		}
		buf.WriteString("    ],\n")
	}
	buf.WriteString("};\n")
	return buf.Bytes()
}

// LoadPacks reads materialized permission-pack.jsonc files from the staging
// directory's .opencode/sys-scripts/permission-packs/ directory. Each active
// overlay pack materializes its self-describing permission descriptor there
// during renderSeamStaging. Returns an empty slice (not nil) when the directory
// is absent (core-only repo, no overlays).
func LoadPacks(stagingDir string) ([]Pack, error) {
	packsDir := filepath.Join(stagingDir, ".opencode", "sys-scripts", "permission-packs")
	entries, err := os.ReadDir(packsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("permconfig: read permission-packs: %w", err)
	}
	var packs []Pack
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".jsonc") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(packsDir, name))
		if err != nil {
			return nil, fmt.Errorf("permconfig: read pack %s: %w", name, err)
		}
		pack, err := parsePack(data, strings.TrimSuffix(name, ".jsonc"))
		if err != nil {
			return nil, fmt.Errorf("permconfig: parse pack %s: %w", name, err)
		}
		packs = append(packs, pack)
	}
	return packs, nil
}

// parsePack parses a permission-pack.jsonc into a Pack. The schema:
//
//	{
//	  "agents": {
//	    "<name>": {
//	      "location": {"wildcard":"deny","readonly":"allow","git_readonly":"allow","gate":"deny","devSh":"allow"},
//	      "task": {"*":"deny","build":"allow"},
//	      "gateExempt": true,
//	      "delegateFrom": ["build","coordination"]
//	    }
//	  }
//	}
func parsePack(data []byte, name string) (Pack, error) {
	m, err := jsonc.Parse(data)
	if err != nil {
		return Pack{}, err
	}
	pack := Pack{Name: name, Agents: map[string]PackAgent{}}
	agentsRaw, ok := m["agents"].(map[string]any)
	if !ok {
		return pack, nil // no agents → empty pack (valid)
	}
	for agentName, agentRaw := range agentsRaw {
		agentMap, ok := agentRaw.(map[string]any)
		if !ok {
			continue
		}
		var agent PackAgent
		if loc, ok := agentMap["location"].(map[string]any); ok {
			rule, err := parseLocation(loc)
			if err != nil {
				return Pack{}, fmt.Errorf("pack %q agent %q: %w", name, agentName, err)
			}
			agent.Location = rule
		}
		if task, ok := agentMap["task"].(map[string]any); ok {
			agent.Task = parseTaskEntries(task)
		}
		if ge, ok := agentMap["gateExempt"].(bool); ok {
			agent.GateExempt = ge
		}
		if df, ok := agentMap["delegateFrom"].([]any); ok {
			for _, d := range df {
				if s, ok := d.(string); ok {
					agent.DelegateFrom = append(agent.DelegateFrom, s)
				}
			}
		}
		pack.Agents[agentName] = agent
	}
	return pack, nil
}

// parseLocation converts a JSON object with wildcard/readonly/git_readonly/
// gate/harnessPolicy/harness/devSh keys into a LocationRule. The gate key is
// optional: its absence means HasGate=false (the agent is gate-exempt or the
// pack author chose to omit it).
//
// The harness policy is read from three mutually-compatible declaration
// channels, all of which accept all four values (allow, ask, deny, read_only):
//
//   - "harnessPolicy" (NEW, PREFERRED — the dedicated policy key).
//   - "harness" (legacy scalar).
//   - "devSh" (deprecated alias of "harness").
//
// If multiple channels are present they must all resolve to the same
// (policy, scalar) pair; specifying channels with different decisions is a
// hard error (fail-closed). The normalized HarnessPolicy is stored on the
// rule; Harness and DevSh carry the implied scalar (wildcardDecision) so
// emission and validation read consistent values. "read_only" is expressible
// through any channel — e.g. {"harness": "read_only"} yields
// HarnessPolicyReadOnly — but the preferred spelling is the dedicated
// "harnessPolicy" key.
func parseLocation(m map[string]any) (LocationRule, error) {
	rule := LocationRule{}
	if v, ok := m["wildcard"].(string); ok {
		rule.Wildcard = Decision(v)
	}
	if v, ok := m["readonly"].(string); ok {
		rule.Readonly = Decision(v)
	}
	if v, ok := m["git_readonly"].(string); ok {
		rule.GitReadonly = Decision(v)
	}
	if v, ok := m["gate"].(string); ok {
		rule.Gate = Decision(v)
		rule.HasGate = true
	}
	// Read the harness policy from any of three declaration channels:
	//   - harnessPolicy (NEW, preferred — the only way to express read_only
	//     via the dedicated policy key; also expressible via the legacy keys)
	//   - harness (legacy scalar — accepts allow/ask/deny/read_only values)
	//   - devSh (deprecated alias of harness)
	// All channels accept all four values (allow/ask/deny/read_only) via
	// parseHarnessValue, so a pack author can write "harness": "read_only" and
	// get HarnessPolicyReadOnly. If multiple channels are present they must all
	// resolve to the SAME (policy, scalar) pair; any disagreement is a hard
	// error (fail-closed). The resulting HarnessPolicy is stored on the rule;
	// Harness and DevSh carry the implied scalar (wildcardDecision).
	// normalizeHarnessFields (called in resolveRules for core tables and
	// validate for all rules) re-checks consistency defensively.
	var declaredPolicy HarnessPolicy
	var declaredScalar Decision
	channelsPresent := 0
	for _, key := range []string{"harnessPolicy", "harness", "devSh"} {
		raw, ok := m[key].(string)
		if !ok {
			continue
		}
		policy, scalar, parsedOK := parseHarnessValue(raw)
		if !parsedOK {
			return LocationRule{}, fmt.Errorf(
				"location: %q value %q is not a valid harness decision (allow, ask, deny, read_only)",
				key, raw)
		}
		if channelsPresent == 0 {
			declaredPolicy = policy
			declaredScalar = scalar
		} else if policy != declaredPolicy {
			return LocationRule{}, fmt.Errorf(
				"location: conflicting harness declarations — %q (%q) and a previous channel (%q) specify different decisions; "+
					"use a single channel (harnessPolicy preferred; harness/devSh are legacy scalar aliases)",
				key, raw, string(declaredPolicy))
		}
		channelsPresent++
	}
	if channelsPresent > 0 {
		rule.HarnessPolicy = declaredPolicy
		rule.Harness = declaredScalar
		rule.DevSh = declaredScalar
	}
	if v, ok := m["edit"].(string); ok {
		rule.Edit = Decision(v)
	} else {
		// Overlay packs that omit edit default to Deny (the corpus default for
		// the majority of agents). Core tables always set Edit explicitly.
		rule.Edit = Deny
	}
	if overrides, ok := m["editOverrides"].(map[string]any); ok {
		// Preserve insertion determinism: sort override patterns alphabetically
		// (Go's json.Unmarshal into map[string]any loses key order). Core
		// tables set EditOverrides as a Go slice (order preserved); this branch
		// only affects overlay-sourced overrides, where the committer's single
		// override is the only realistic case.
		keys := make([]string, 0, len(overrides))
		for k := range overrides {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if d, ok := overrides[k].(string); ok {
				rule.EditOverrides = append(rule.EditOverrides, EditRule{Pattern: k, Decision: Decision(d)})
			}
		}
	}
	return rule, nil
}

// parseTaskEntries converts a JSON object (target→decision) into an
// insertion-conventional []TaskEntry. Go's encoding/json loses JSON object key
// order (unmarshals into map[string]any), so entries are sorted alphabetically.
// "*" (ASCII 42) sorts before all lowercase letters, so it is naturally first —
// matching the resolver's "*" -first convention.
func parseTaskEntries(m map[string]any) []TaskEntry {
	entries := make([]TaskEntry, 0, len(m))
	for target, decision := range m {
		d, ok := decision.(string)
		if !ok {
			continue
		}
		entries = append(entries, TaskEntry{Target: target, Decision: Decision(d)})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Target < entries[j].Target
	})
	return entries
}
