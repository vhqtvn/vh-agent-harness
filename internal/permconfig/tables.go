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
		"printf *",
		"sleep *",
		"sed -n *",
		".opencode/scripts/readonly-scripts.sh *",
		// commit-gate.sh status is a PURE-READ metadata probe (cmd_status only
		// reads lock/session metadata and emits JSON); it lives in readonly so
		// ALL agents — including gate-exempt ones (build/coordination/
		// project-coordinator/docs-steward) — get prompt-free lock checks. The
		// mutation verbs (acquire/commit/release/heartbeat/revert/stage-message)
		// stay in the gate group below = committer-only.
		".opencode/scripts/commit-gate.sh status",
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
		// `git --no-pager <sub>` forms are RETAINED as config-table defense-in-
		// depth. shell-guard's `walkGitGlobals` is now the PRIMARY prompt-free
		// path: it strips the `--no-pager` global flag (and `-p` / `--paginate`
		// / `-P`) and rewrites `git --no-pager <sub>` -> `git <sub>` BEFORE the
		// opencode matcher runs, so the bare `git <sub> *` patterns above already
		// cover the rewritten form. These explicit entries are belt-and-
		// suspenders for any caller that bypasses the plugin. The mutation-slip
		// guard for `git --no-pager commit` lives in shell-guard-core.js
		// (walkGitGlobals verb extraction), not here.
		"git --no-pager diff *",
		"git --no-pager log *",
		"git --no-pager show *",
		"git --no-pager grep *",
		"git --no-pager blame *",
		"git --no-pager ls-tree *",
		"git --no-pager status *",
		"git --no-pager ls-files *",
		"git --no-pager check-ignore *",
		"git --no-pager cat-file *",
		"git --no-pager show-ref *",
		"git --no-pager rev-parse *",
	}},
	{Name: "gate", Commands: []string{
		".opencode/scripts/commit-gate.sh acquire *",
		".opencode/scripts/commit-gate.sh commit *",
		".opencode/scripts/commit-gate.sh release *",
		".opencode/scripts/commit-gate.sh heartbeat *",
		".opencode/scripts/commit-gate.sh revert *",
		".opencode/scripts/commit-gate.sh stage-message *",
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
//
// Edit values mirror the corpus template's flat edit decisions EXACTLY so that
// bringing edit under emitter ownership is a no-op for every flat-edit agent:
// default=ask, all others=deny, EXCEPT as noted below for the three
// object-form agents.
//
// Three agents carry EditOverrides (object-form edit, findLast semantics —
// permission/evaluate.ts picks the LAST matching pattern):
//
//   - committer: Edit=Deny + scoped allow for tmp/commit-gate-message/** so it
//     can Write the commit message file that acquire --message-file consumes.
//     Pattern: BROAD DENY + NARROW ALLOW.
//   - build and docs-steward: Edit=Allow + scoped deny for
//     docs/planning/backlog.md (BacklogPromoterDenyPath). This is the W1
//     single-writer-promotion enforcement seam: worker agents cannot directly
//     edit the canonical backlog, forcing status updates through
//     .local/coordinator/tasks/ as status transport. Only a designated
//     promoter (initially the operator via coordination) writes the backlog.
//     Pattern: BROAD ALLOW + NARROW DENY — the inverse of the committer.
//
// Every other agent stays flat edit. Do not add more EditOverrides without an
// explicit safety review of the findLast interaction (key order is load-
// bearing: "*" first, overrides last).
var CoreLocationRules = map[string]LocationRule{
	"default": {Wildcard: Deny, Readonly: Allow, GitReadonly: Allow, Gate: Deny, HasGate: true, DevSh: Allow, Edit: Ask},
	"plan":    {Wildcard: Deny, Readonly: Allow, GitReadonly: Allow, Gate: Deny, HasGate: true, DevSh: Ask, Edit: Deny},
	"build": {
		Wildcard: Ask, Readonly: Allow, GitReadonly: Allow, HasGate: false, DevSh: Allow, Edit: Allow, // gate-exempt
		// Object-form edit: allow-* FIRST, then the W1 backlog deny LAST.
		// findLast picks the narrow deny for docs/planning/backlog.md; every
		// other path resolves to the broad allow. This blocks build from
		// directly editing the canonical backlog, forcing status updates
		// through .local/coordinator/tasks/ for promoter-mediated writes.
		EditOverrides: []EditRule{{Pattern: BacklogPromoterDenyPath, Decision: Deny}},
	},
	"coordination":        {Wildcard: Deny, Readonly: Allow, GitReadonly: Allow, HasGate: false, DevSh: Allow, Edit: Deny}, // gate-exempt
	"planner":             {Wildcard: Deny, Readonly: Allow, GitReadonly: Allow, Gate: Deny, HasGate: true, DevSh: Allow, Edit: Deny},
	"researcher":          {Wildcard: Deny, Readonly: Allow, GitReadonly: Allow, Gate: Deny, HasGate: true, DevSh: Allow, Edit: Deny},
	"project-coordinator": {Wildcard: Deny, Readonly: Allow, GitReadonly: Allow, HasGate: false, DevSh: Allow, Edit: Deny}, // gate-exempt
	"debate":              {Wildcard: Deny, Readonly: Allow, GitReadonly: Allow, Gate: Deny, HasGate: true, DevSh: Deny, Edit: Deny},
	"debate-proposer":     {Wildcard: Deny, Readonly: Allow, GitReadonly: Allow, Gate: Deny, HasGate: true, DevSh: Deny, Edit: Deny},
	"debate-critic":       {Wildcard: Deny, Readonly: Allow, GitReadonly: Allow, Gate: Deny, HasGate: true, DevSh: Deny, Edit: Deny},
	"debate-synth":        {Wildcard: Deny, Readonly: Allow, GitReadonly: Allow, Gate: Deny, HasGate: true, DevSh: Deny, Edit: Deny},
	"solution-brief":      {Wildcard: Deny, Readonly: Allow, GitReadonly: Allow, Gate: Deny, HasGate: true, DevSh: Deny, Edit: Deny},
	"repo-explorer":       {Wildcard: Deny, Readonly: Allow, GitReadonly: Allow, Gate: Deny, HasGate: true, DevSh: Ask, Edit: Deny},
	"docs-steward": {
		Wildcard: Deny, Readonly: Allow, GitReadonly: Allow, HasGate: false, DevSh: Ask, Edit: Allow, // gate-exempt
		// Object-form edit: same W1 single-writer-promotion model as build —
		// allow-* FIRST, then the backlog deny LAST. docs-steward owns durable
		// repo GUIDANCE (AGENTS.md, docs/ai/, docs/checkpoints/) but NOT the
		// canonical backlog, which stays promoter-mediated. See build's
		// comment and BacklogPromoterDenyPath for the rationale.
		EditOverrides: []EditRule{{Pattern: BacklogPromoterDenyPath, Decision: Deny}},
	},
	"commit-message":  {Wildcard: Deny, Readonly: Allow, GitReadonly: Allow, Gate: Deny, HasGate: true, DevSh: Allow, Edit: Deny},
	"commit-reviewer": {Wildcard: Deny, Readonly: Allow, GitReadonly: Allow, Gate: Deny, HasGate: true, DevSh: Allow, Edit: Deny},
	"committer": {
		Wildcard: Deny, Readonly: Allow, GitReadonly: Allow, Gate: Allow, HasGate: true, DevSh: Deny,
		// Object-form edit: deny-* FIRST, then the ONE scoped allow LAST. The
		// committer authors its commit message via the Write tool at
		// tmp/commit-gate-message/msg-${UUID}, which acquire --message-file
		// consumes. findLast picks the narrow allow for that path; every other
		// path denies. Pattern: broad deny + narrow allow (the inverse of
		// build/docs-steward's broad allow + narrow deny for the W1 backlog).
		Edit:          Deny,
		EditOverrides: []EditRule{{Pattern: CommitGateMessageGlob, Decision: Allow}},
	},
	"ship-review": {Wildcard: Deny, Readonly: Allow, GitReadonly: Allow, Gate: Deny, HasGate: true, DevSh: Allow, Edit: Deny},
	// Cluster leaves (commit-reviewer-a..d) — the corpus ships these as full
	// agent blocks. They carry the leafBaseRule (deny wildcard, allow
	// readonly/git_readonly, deny gate, allow devSh) and a deny-all task rule.
	"commit-reviewer-a": {Wildcard: Deny, Readonly: Allow, GitReadonly: Allow, Gate: Deny, HasGate: true, DevSh: Allow, Edit: Deny},
	"commit-reviewer-b": {Wildcard: Deny, Readonly: Allow, GitReadonly: Allow, Gate: Deny, HasGate: true, DevSh: Allow, Edit: Deny},
	"commit-reviewer-c": {Wildcard: Deny, Readonly: Allow, GitReadonly: Allow, Gate: Deny, HasGate: true, DevSh: Allow, Edit: Deny},
	"commit-reviewer-d": {Wildcard: Deny, Readonly: Allow, GitReadonly: Allow, Gate: Deny, HasGate: true, DevSh: Allow, Edit: Deny},
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

// CommitGateMessageGlob is the ONE scoped edit-tool path the committer may
// Write to. It is repo-relative (the edit tool passes path.relative(worktree,
// filePath)) and uses the recursive ** glob. The committer authors its commit
// message at tmp/commit-gate-message/msg-${UUID} via the Write tool, then
// passes that path to commit-gate.sh acquire --message-file. tmp/ is gitignored
// so the message file never enters the index. This is the committer's edit
// widening (broad deny + narrow allow); the inverse pattern (broad allow +
// narrow deny) lives on build/docs-steward via BacklogPromoterDenyPath. Do not
// add more EditOverrides without an explicit safety review of the findLast
// interaction.
const CommitGateMessageGlob = "tmp/commit-gate-message/**"

// BacklogPromoterDenyPath is the canonical-backlog path that worker agents
// (build, docs-steward) are DENIED from editing directly. It is the W1
// single-writer-promotion enforcement seam: blocking worker edits forces status
// updates through .local/coordinator/tasks/ as status transport, so only a
// designated promoter (initially the operator via coordination) writes
// docs/planning/backlog.md. The path is repo-relative and exact (no glob) —
// the edit tool passes path.relative(worktree, filePath), and this matches
// that single canonical file. Pair with Edit=Allow on the agent so the
// wildcard broad decision stays allow (findLast picks the narrow deny for
// this path; every other path resolves to allow). This is the inverse of the
// committer's CommitGateMessageGlob pattern (broad deny + narrow allow); here
// it is broad allow + narrow deny.
const BacklogPromoterDenyPath = "docs/planning/backlog.md"
