package permconfig

import "strings"

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
		// exec-ro is the script-level STRICTLY READ-ONLY execution verb
		// (internal/execro classifier + internal/cli/exec_ro.go). It is allowlisted
		// here as `vh-agent-harness exec-ro *` so opencode NEVER prompts for it —
		// which means exec-ro itself is the ONLY gate for its payload and MUST
		// hard-deny dangerous inner commands (mutations, out-of-repo reads,
		// shell metacharacters). Adding it to the readonly group gives EVERY agent
		// prompt-free read-only inspection (safe: exec-ro denies all mutations
		// regardless of caller, so even the committer having it is harmless).
		"vh-agent-harness exec-ro *",
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
		// `git --no-pager <sub>` forms are the PRIMARY config-table prompt-free
		// path for `--no-pager` readonly invocations. shell-guard's
		// `walkGitGlobals` classifies these commands for the security DECISION
		// (mutation-slip guard for `git --no-pager commit`) but does NOT rewrite
		// the command — the plugin is detect/parse-only by design, because
		// rewriting the command string would mutate EXECUTION and shell syntax
		// (pipelines, subshells, sequences, redirects) makes that unsafe. So
		// without these explicit entries, `git --no-pager <sub>` would prompt
		// (the bare `git <sub> *` patterns above only match a stripped form that
		// no longer happens). The mutation-slip guard lives in shell-guard-core.js
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

// GitMutationVerbs is the SINGLE CANONICAL SOURCE for the set of git verbs that
// mutate repo state and are therefore forbidden to every agent except the
// committer (via commit-gate.sh). This Go slice owns the list; it is emitted to
// allowed-commands.js (GenerateAllowedCommandsJS) as `GIT_MUTATION_VERBS` and
// consumed by BOTH:
//
//   - the exec-ro classifier (internal/execro.Classify) — to deny git mutations
//     routed past any leading global flag (`git -C <ext> commit`,
//     `git --no-pager commit`, `git --git-dir=/x commit`);
//   - shell-guard-core.js — which imports the Go-generated `GIT_MUTATION_VERBS`
//     from allowed-commands.js (NOT a hardcoded JS array) to build the
//     `git-mutation-bypass` regex and power its mutation-slip guard.
//
// Single-source discipline: do NOT inline this alternation anywhere else. The
// previous JS-only definition lived in forbidden-patterns.core.js; it moved here
// so Go (exec-ro) and JS (shell-guard) cannot drift. Keep this list in sync with
// the verbs the commit-gate wrapper accepts (the committer's mutation surface).
var GitMutationVerbs = []string{
	"add", "commit", "push", "reset", "commit-tree", "update-ref",
	"checkout", "merge", "rebase", "stash", "branch", "restore",
	"cherry-pick", "revert", "clean", "rm", "mv", "tag", "am", "apply", "switch",
}

// GitMutationVerbsSet is the set form of GitMutationVerbs for O(1) membership
// lookup by the exec-ro classifier (and any Go consumer). Built once at init;
// the slice above stays the canonical ordered source for JS emission.
var GitMutationVerbsSet = func() map[string]bool {
	m := make(map[string]bool, len(GitMutationVerbs))
	for _, v := range GitMutationVerbs {
		m[v] = true
	}
	return m
}()

// GitReadonlyVerbs is the set of git verbs recognized as read-only inspection,
// derived from the `git_readonly` command group (the `<verb>` in `git <verb> *`).
// The exec-ro classifier tests the walker-extracted verb against this set: a
// readonly verb ALLOWs (subject to `-C`/`--git-dir` path classification); any
// other verb (mutation OR unknown) DEFAULT-DENIES. Paging flags (`--no-pager`)
// are stripped by the walker before verb extraction, so `git --no-pager log`
// yields verb `log` and matches here — the explicit `git --no-pager <sub> *`
// entries in the config table are NOT consulted by exec-ro (the walker handles
// them). Built once at package init from CommandGroups so there is no second
// hand-maintained list.
var GitReadonlyVerbs = func() map[string]bool {
	m := map[string]bool{}
	for _, cmd := range groupByName("git_readonly") {
		// Each entry is `git <verb> *` or `git --no-pager <verb> *`. Extract the
		// verb: the token after `git`, skipping a leading `--no-pager`.
		parts := splitFields(cmd)
		if len(parts) < 2 || parts[0] != "git" {
			continue
		}
		verb := parts[1]
		if verb == "--no-pager" && len(parts) >= 3 {
			verb = parts[2]
		}
		if verb != "" && verb != "*" {
			m[verb] = true
		}
	}
	return m
}()

// groupByName returns the command list of one group, or nil if absent.
func groupByName(name string) []string {
	for _, g := range CommandGroups {
		if g.Name == name {
			return g.Commands
		}
	}
	return nil
}

// splitFields splits s on runs of whitespace (no quote handling — used only on
// the curated command-table literals, which contain no spaces inside tokens).
func splitFields(s string) []string {
	return strings.Fields(s)
}

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
// Edit values mirror the corpus template's edit decisions, extended by a
// UNIVERSAL disposable-scratch carve-out: every agent may Write the gitignored,
// watcher-ignored `tmp/**` scratch surface (see TmpWriteGlob). The emitter's
// computeEditBlock appends `tmp/**: allow` as the LAST entry of every
// object-form edit block, so it overrides the broad deny/ask for tmp paths
// while leaving every OTHER edit decision exactly as it was. This is the
// single chokepoint that lets read-only agents (and overlay-pack agents that
// lack an edit key, like the releaser) drop disposable artifacts under tmp/
// without a permission prompt — tmp/ is the sanctioned scratch area, never
// committed, never watched.
//
// Resulting shapes (findLast semantics — permission/evaluate.ts picks the LAST
// matching pattern; key order is load-bearing: broad decision FIRST, every
// narrow allow LAST):
//
//   - build / docs-steward: flat "allow" (tmp is already covered by their broad
//     allow — no object form needed, computeEditBlock skips the carve-out).
//   - committer: {"*":"deny", "tmp/commit-gate-message/**":"allow",
//     "tmp/**":"allow"} — its ONE EditOverride (the commit-gate message glob)
//     plus the universal tmp carve-out.
//   - top-level default: {"*":"ask", "tmp/**":"allow"}.
//   - every other (read-only) agent: {"*":"deny", "tmp/**":"allow"}.
//
// build and docs-steward carry a BROAD flat Edit=Allow — agents edit
// docs/planning/backlog.md freely. Backlog conflict discipline is enforced at
// the commit/workflow layer (the `backlog` skill + non-blocking reminder
// plugin), NOT by blocking edits here. See BacklogLedgerPath for the canonical
// path constant shared with the reminder plugin + cross-constant test.
//
// The committer is the only agent that carries EditOverrides today (its scoped
// commit-gate message glob). The tmp/** carve-out is added UNCONDITIONALLY in
// computeEditBlock for every non-flat-allow agent — it is NOT an EditOverride
// in this table. Do not add more EditOverrides without an explicit safety
// review of the findLast interaction (key order is load-bearing: "*" first,
// then EditOverrides, then tmp/** last).
var CoreLocationRules = map[string]LocationRule{
	"default": {Wildcard: Deny, Readonly: Allow, GitReadonly: Allow, Gate: Deny, HasGate: true, DevSh: Allow, Edit: Ask},
	"plan":    {Wildcard: Deny, Readonly: Allow, GitReadonly: Allow, Gate: Deny, HasGate: true, DevSh: Ask, Edit: Deny},
	"build": {
		Wildcard: Ask, Readonly: Allow, GitReadonly: Allow, HasGate: false, DevSh: Allow, Edit: Allow, // gate-exempt
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
	},
	"commit-message":  {Wildcard: Deny, Readonly: Allow, GitReadonly: Allow, Gate: Deny, HasGate: true, DevSh: Allow, Edit: Deny},
	"commit-reviewer": {Wildcard: Deny, Readonly: Allow, GitReadonly: Allow, Gate: Deny, HasGate: true, DevSh: Allow, Edit: Deny},
	"committer": {
		Wildcard: Deny, Readonly: Allow, GitReadonly: Allow, Gate: Allow, HasGate: true, DevSh: Deny,
		// Object-form edit: deny-* FIRST, then the ONE scoped allow LAST. The
		// committer authors its commit message via the Write tool at
		// tmp/commit-gate-message/msg-${UUID}, which acquire --message-file
		// consumes. findLast picks the narrow allow for that path; every other
		// path denies. The committer is the SOLE agent carrying EditOverrides
		// now (build/docs-steward reverted to broad flat Edit=Allow when the W1
		// single-writer edit-blocking was unwound in favor of commit-layer
		// conflict discipline).
		Edit:          Deny,
		EditOverrides: []EditRule{{Pattern: CommitGateMessageGlob, Decision: Allow}},
	},
	"ship-review": {Wildcard: Deny, Readonly: Allow, GitReadonly: Allow, Gate: Deny, HasGate: true, DevSh: Allow, Edit: Deny},
	// media-perception: opt-in read-only perception specialist
	// (core/media-perception). Rendered only when the capability is selected;
	// the four inbound caller edges below are dropped by Emit's present-agent
	// filter when this agent is absent from the rendered roster. NOT
	// gate-exempt — gate-exempt is reserved for the four orchestrators above.
	"media-perception": {Wildcard: Deny, Readonly: Allow, GitReadonly: Allow, Gate: Deny, HasGate: true, DevSh: Allow, Edit: Deny},
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
		// media-perception: dropped by Emit's present-agent filter when the
		// capability is unselected (the agent block is not rendered).
		{"media-perception", Allow},
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
		// media-perception: dropped by Emit's present-agent filter when the
		// capability is unselected.
		{"media-perception", Allow},
	},
	"planner": {
		{"*", Deny},
	},
	"researcher": {
		{"*", Deny},
		// media-perception: single outbound edge for an otherwise-read-only
		// leaf, so a researcher holding a media locator can delegate
		// perception to the specialist. Dropped by Emit's present-agent
		// filter when the capability is unselected.
		{"media-perception", Allow},
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
	// media-perception: read-only perception leaf. Deny-all task; no
	// outbound delegation. Rendered only when core/media-perception is
	// selected (present-agent filter drops the inbound edges otherwise).
	"media-perception": {
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
		// media-perception: dropped by Emit's present-agent filter when the
		// capability is unselected.
		{"media-perception", Allow},
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
// Write to (in addition to the universal TmpWriteGlob carve-out). It is
// repo-relative (the edit tool passes path.relative(worktree, filePath)) and
// uses the recursive ** glob. The committer authors its commit message at
// tmp/commit-gate-message/msg-${UUID} via the Write tool, then passes that
// path to commit-gate.sh acquire --message-file. tmp/ is gitignored so the
// message file never enters the index. The committer is the SOLE agent
// carrying EditOverrides (broad deny + narrow allows); build/docs-steward
// reverted to broad flat Edit=Allow when the W1 single-writer edit-blocking
// was unwound. Do not add more EditOverrides without an explicit safety review
// of the findLast interaction.
const CommitGateMessageGlob = "tmp/commit-gate-message/**"

// TmpWriteGlob is the UNIVERSAL disposable-scratch carve-out: every agent may
// Write paths under `tmp/**` with no permission prompt. tmp/ is gitignored
// (.gitignore) and watcher-ignored, so it is the sanctioned disposable scratch
// surface — run artifacts, scratch scripts, captured traces, message files all
// live there and never enter the index or trigger a watch rebuild. It is
// repo-relative and uses the recursive ** glob (matching the edit-tool
// path-relative convention). computeEditBlock appends this as the LAST entry of
// every object-form edit block so findLast resolves tmp paths to allow; flat-
// allow agents (build, docs-steward) skip it because their broad allow already
// covers tmp. This is the single chokepoint that covers overlay-pack agents
// that lack an edit key (e.g. the releaser) — they inherit the top-level
// default's {"*":"ask","tmp/**":"allow"} and so can drop release-tag message
// files under tmp/ without a prompt.
const TmpWriteGlob = "tmp/**"

// BacklogLedgerPath is the canonical task-status ledger path. It is the shared
// "backlog path" reference point for the consumers that must stay aligned with
// it: (1) this permconfig table (historical context for the W1 edit-blocking
// that has been unwound — agents now edit the backlog freely, with conflict
// discipline enforced at the commit/workflow layer instead); (2) the
// commit-gate.sh O1 packaging-policy preflight, which refuses an acquire whose
// --paths mixes this ledger with code/docs changes (the literal
// docs/planning/backlog.md lives in that shell guard; this Go constant is the
// canonical reference for any Go-side consumer); (3) the backlog skill and the
// promoter runbook's eventual-consistency pass, which cite this path as the
// canonical status ledger. The path is repo-relative and exact (no glob).
const BacklogLedgerPath = "docs/planning/backlog.md"
