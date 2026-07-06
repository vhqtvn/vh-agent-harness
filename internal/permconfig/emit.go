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
	root, err := jsonc.Parse(input)
	if err != nil {
		return nil, fmt.Errorf("permconfig: parse opencode.jsonc: %w", err)
	}

	// Resolve the active rule set: deep-copy core tables, merge overlay packs
	// (overlay agent location/task rules, gateExempt, delegateFrom edges).
	locations, tasks, gateExempt := resolveRules(packs)

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

	// present is the set of agents that have a block in the rendered config.
	// Task edges to absent agents — gated out of the template by a capability
	// gate (e.g. committer when core/gated-commit is unselected) — are dropped
	// from every orchestrator's task allowlist below. This keeps permconfig.Emit
	// coherent with the template: no agent may delegate to an agent that isn't
	// there. See Phase 3 capability gating in internal/cli/profile.go
	// (resolveCapabilityAnswers) and the cluster gates in opencode.jsonc.tmpl.
	agents, _ := root["agent"].(map[string]any)
	present := make(map[string]bool, len(agents))
	for name := range agents {
		present[name] = true
	}

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

// resolveRules deep-copies the core tables and merges overlay pack
// contributions: overlay agent location/task rules replace core entries (or
// add new agents), gateExempt flags extend the base set, and delegateFrom
// edges inject "{agentName}: allow" into each declaring orchestrator's task
// allowlist (appended after core entries, matching the resolver's insertion
// semantics).
func resolveRules(packs []Pack) (locations map[string]LocationRule, tasks map[string][]TaskEntry, gateExempt map[string]bool) {
	locations = make(map[string]LocationRule, len(CoreLocationRules))
	for k, v := range CoreLocationRules {
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
//     this same sort.
//  3. "vh-agent-harness *" (devSh decision) — always LAST.
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

	om.set(DevShCommand, string(rule.DevSh))
	return om
}

// computeEditBlock renders the permission.edit value for one location. When
// EditOverrides is empty, edit is flat: a single decision string ("allow",
// "deny", "ask") — the common case for every agent except the committer. When
// EditOverrides is non-empty, edit is an OBJECT map {"<pattern>": "<action>"}
// consumed by OpenCode with findLast semantics (permission/evaluate.ts): the
// "*" entry (carrying the Edit decision, typically Deny) is emitted FIRST and
// each override LAST, so a path matching a narrow allow resolves to allow while
// everything else denies. This is how the committer gets Write access to ONE
// scoped message-file path (tmp/commit-gate-message/**) and nothing else.
func computeEditBlock(rule LocationRule) any {
	if len(rule.EditOverrides) == 0 {
		return string(rule.Edit)
	}
	om := newOrderedMap()
	// Deny-* FIRST. findLast picks the LAST matching rule, so the broad deny
	// must precede every narrow allow.
	om.set("*", string(rule.Edit))
	// Narrow allows LAST — they override the deny for matching paths.
	for _, o := range rule.EditOverrides {
		om.set(o.Pattern, string(o.Decision))
	}
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
			agent.Location = parseLocation(loc)
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
// gate/devSh keys into a LocationRule. The gate key is optional: its absence
// means HasGate=false (the agent is gate-exempt or the pack author chose to
// omit it).
func parseLocation(m map[string]any) LocationRule {
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
	if v, ok := m["devSh"].(string); ok {
		rule.DevSh = Decision(v)
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
	return rule
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
