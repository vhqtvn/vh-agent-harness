package permconfig

// This file is the SINGLE CANONICAL SOURCE for the permission content that
// ships into opencode.jsonc. The legacy Node resolver
// (templates/core/.opencode/sys-scripts/update-opencode-config.js) previously
// held a parallel copy of this data in its CORE_LOCATION_RULES / CORE_TASK_RULES
// / GATE_EXEMPT_AGENTS tables and the allowed-commands.js command groups; that
// dual maintenance is what P2-PIPELINE-001 collapses. These Go tables ARE the
// authority now; the resolver is demoted to a migration aid.
//
// When editing a permission decision here, you are changing what every install
// ships. The corpus template (templates/core/opencode.jsonc.tmpl) carries a
// pre-rendered copy of these literals as a self-describing scaffold, but the
// emitter OVERWRITES those blocks from these tables at build time, so the Go
// tables are what doctor compares against.

// CommandGroup is one named group of shell commands and the shared decision
// applied to all of them in a permission.bash block.
type CommandGroup struct {
	Name     string
	Commands []string
}

// CommandGroups is the canonical command roster, in the iteration order the
// resolver used (readonly, git_readonly, gate). This order matters only for
// VALIDATION iteration; the emitted bash block independently sorts commands by
// length-then-locale. The `custom` group from the legacy resolver is EMPTY and
// omitted (it was never populated and shell-guard does not consume it).
var CommandGroups = []CommandGroup{
	{Name: "readonly", Commands: []string{
		"true *",
		"cut *",
		"wc *",
		"ls *",
		"cat *",
		"head *",
		"tail *",
		"sort *",
		"uniq *",
		"grep *",
		"date *",
		"rg *",
		"jq *",
		"find *",
		"echo *",
		"sleep *",
		"sed -n *",
		".opencode/scripts/readonly-scripts.sh *",
	}},
	{Name: "git_readonly", Commands: []string{
		"git diff *",
		"git log *",
		"git show *",
		"git grep *",
		"git blame *",
		"git ls-tree *",
		"git status *",
		"git ls-files *",
		"git check-ignore *",
		"git cat-file *",
		"git show-ref *",
		"git rev-parse *",
	}},
	{Name: "gate", Commands: []string{
		".opencode/scripts/commit-gate.sh acquire *",
		".opencode/scripts/commit-gate.sh commit *",
		".opencode/scripts/commit-gate.sh release *",
		".opencode/scripts/commit-gate.sh heartbeat *",
		".opencode/scripts/commit-gate.sh revert *",
		".opencode/scripts/commit-gate.sh stage-message *",
		".opencode/scripts/commit-gate.sh status",
		"uuidgen",
	}},
}

// GroupNames lists the group names in canonical iteration order, for consumers
// that need to enumerate groups (e.g. diagnostics, future JSON-schema emission).
var GroupNames = []string{"readonly", "git_readonly", "gate"}

// CoreLocationRules maps each agent location name (plus "default" for the
// top-level permission.bash block) to its bash decisions. Gate-exempt agents
// (build, coordination, project-coordinator, docs-steward) have HasGate=false
// so the gate key is OMITTED from their emitted bash block — this is the
// safety contract: OpenCode's deriveSubagentSessionPermission merges parent
// denies into subagent sessions via findLast, so a parent gate deny would
// override the committer's gate allow, breaking the gated-commit protocol.
//
// The committer is the ONLY agent with Gate=Allow (it commits through the
// gate wrapper). Every other gate-present agent has Gate=Deny.
var CoreLocationRules = map[string]LocationRule{
	"default":             {Wildcard: Deny, Readonly: Allow, GitReadonly: Allow, Gate: Deny, HasGate: true, DevSh: Allow},
	"plan":                {Wildcard: Deny, Readonly: Allow, GitReadonly: Allow, Gate: Deny, HasGate: true, DevSh: Ask},
	"build":               {Wildcard: Ask, Readonly: Allow, GitReadonly: Allow, HasGate: false, DevSh: Allow},  // gate-exempt
	"coordination":        {Wildcard: Deny, Readonly: Allow, GitReadonly: Allow, HasGate: false, DevSh: Allow}, // gate-exempt
	"planner":             {Wildcard: Deny, Readonly: Allow, GitReadonly: Allow, Gate: Deny, HasGate: true, DevSh: Allow},
	"researcher":          {Wildcard: Deny, Readonly: Allow, GitReadonly: Allow, Gate: Deny, HasGate: true, DevSh: Allow},
	"project-coordinator": {Wildcard: Deny, Readonly: Allow, GitReadonly: Allow, HasGate: false, DevSh: Allow}, // gate-exempt
	"debate":              {Wildcard: Deny, Readonly: Allow, GitReadonly: Allow, Gate: Deny, HasGate: true, DevSh: Deny},
	"debate-proposer":     {Wildcard: Deny, Readonly: Allow, GitReadonly: Allow, Gate: Deny, HasGate: true, DevSh: Deny},
	"debate-critic":       {Wildcard: Deny, Readonly: Allow, GitReadonly: Allow, Gate: Deny, HasGate: true, DevSh: Deny},
	"debate-synth":        {Wildcard: Deny, Readonly: Allow, GitReadonly: Allow, Gate: Deny, HasGate: true, DevSh: Deny},
	"solution-brief":      {Wildcard: Deny, Readonly: Allow, GitReadonly: Allow, Gate: Deny, HasGate: true, DevSh: Deny},
	"repo-explorer":       {Wildcard: Deny, Readonly: Allow, GitReadonly: Allow, Gate: Deny, HasGate: true, DevSh: Ask},
	"docs-steward":        {Wildcard: Deny, Readonly: Allow, GitReadonly: Allow, HasGate: false, DevSh: Ask}, // gate-exempt
	"commit-message":      {Wildcard: Deny, Readonly: Allow, GitReadonly: Allow, Gate: Deny, HasGate: true, DevSh: Allow},
	"commit-reviewer":     {Wildcard: Deny, Readonly: Allow, GitReadonly: Allow, Gate: Deny, HasGate: true, DevSh: Allow},
	"committer":           {Wildcard: Deny, Readonly: Allow, GitReadonly: Allow, Gate: Allow, HasGate: true, DevSh: Deny},
	"ship-review":         {Wildcard: Deny, Readonly: Allow, GitReadonly: Allow, Gate: Deny, HasGate: true, DevSh: Allow},
	// Cluster leaves (commit-reviewer-a..d) — the corpus ships these as full
	// agent blocks. They carry the leafBaseRule (deny wildcard, allow
	// readonly/git_readonly, deny gate, allow devSh) and a deny-all task rule.
	"commit-reviewer-a": {Wildcard: Deny, Readonly: Allow, GitReadonly: Allow, Gate: Deny, HasGate: true, DevSh: Allow},
	"commit-reviewer-b": {Wildcard: Deny, Readonly: Allow, GitReadonly: Allow, Gate: Deny, HasGate: true, DevSh: Allow},
	"commit-reviewer-c": {Wildcard: Deny, Readonly: Allow, GitReadonly: Allow, Gate: Deny, HasGate: true, DevSh: Allow},
	"commit-reviewer-d": {Wildcard: Deny, Readonly: Allow, GitReadonly: Allow, Gate: Deny, HasGate: true, DevSh: Allow},
}

// GateExemptBase is the set of agents that must NOT carry a gate key in their
// bash block. Overlay packs can contribute additional gate-exempt agents via
// permission-pack.jsonc gateExempt:true; the resolved set starts from this base.
var GateExemptBase = map[string]bool{
	"build":               true,
	"coordination":        true,
	"project-coordinator": true,
	"docs-steward":        true,
}

// CoreTaskRules maps each agent to its permission.task allowlist, in the
// insertion order the emitter renders (the resolver used Object.entries which
// preserves declaration order; these slices encode that order explicitly).
// The first entry is always the "*" wildcard. Orchestrators (build,
// coordination, project-coordinator) list every agent they may delegate to;
// leaf agents deny everything except "*".
//
// The commit-reviewer cluster edges (commit-reviewer → a/b/c/d) are included
// directly here since the cluster is static at 4 leaves.
var CoreTaskRules = map[string][]TaskEntry{
	"plan": {
		{"*", Deny},
	},
	"build": {
		{"*", Deny},
		{"commit-message", Allow},
		{"project-coordinator", Allow},
		{"planner", Allow},
		{"researcher", Allow},
		{"repo-explorer", Allow},
		{"commit-reviewer", Allow},
		{"ship-review", Allow},
		{"committer", Allow},
		{"docs-steward", Allow},
		{"debate", Allow},
		{"solution-brief", Allow},
	},
	"coordination": {
		{"*", Deny},
		{"build", Allow},
		{"project-coordinator", Allow},
		{"commit-message", Allow},
		{"planner", Allow},
		{"researcher", Allow},
		{"repo-explorer", Allow},
		{"commit-reviewer", Allow},
		{"ship-review", Allow},
		{"committer", Allow},
		{"debate", Allow},
		{"solution-brief", Allow},
	},
	"planner": {
		{"*", Deny},
	},
	"researcher": {
		{"*", Deny},
	},
	"repo-explorer": {
		{"*", Deny},
	},
	"commit-reviewer": {
		{"*", Deny},
		// Cluster leaf edges: commit-reviewer may delegate to its tier-cascade leaves.
		{"commit-reviewer-a", Allow},
		{"commit-reviewer-b", Allow},
		{"commit-reviewer-c", Allow},
		{"commit-reviewer-d", Allow},
	},
	"ship-review": {
		{"*", Deny},
	},
	"project-coordinator": {
		{"*", Deny},
		{"build", Allow},
		{"commit-message", Allow},
		{"planner", Allow},
		{"researcher", Allow},
		{"repo-explorer", Allow},
		{"commit-reviewer", Allow},
		{"ship-review", Allow},
		{"committer", Allow},
		{"debate", Allow},
		{"solution-brief", Allow},
	},
	"debate": {
		{"*", Deny},
		{"debate-proposer", Allow},
		{"debate-critic", Allow},
		{"debate-synth", Allow},
	},
	"debate-proposer": {
		{"*", Deny},
	},
	"debate-critic": {
		{"*", Deny},
	},
	"debate-synth": {
		{"*", Deny},
	},
	"solution-brief": {
		{"*", Deny},
		{"researcher", Allow},
		{"debate", Allow},
		{"planner", Allow},
	},
	"docs-steward": {
		{"*", Deny},
		{"committer", Allow},
	},
	"commit-message": {
		{"*", Deny},
		{"commit-reviewer", Allow},
	},
	"committer": {
		{"*", Deny},
		{"commit-reviewer", Allow},
	},
	// Cluster leaves — deny-all task.
	"commit-reviewer-a": {{"*", Deny}},
	"commit-reviewer-b": {{"*", Deny}},
	"commit-reviewer-c": {{"*", Deny}},
	"commit-reviewer-d": {{"*", Deny}},
}

// BacklogCommand is the feature-gated bash entry added to the top-level
// permission.bash block when features.backlog is enabled. It participates in
// the same length-then-locale sort as the command-group entries.
const BacklogCommand = "vh-agent-harness exec node .opencode/scripts/normalize-backlog.js"

// DevShCommand is the always-last entry in every bash block, keyed by the
// "vh-agent-harness *" wildcard that matches the binary's own invocations.
const DevShCommand = "vh-agent-harness *"
