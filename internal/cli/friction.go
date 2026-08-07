package cli

// friction.go carries the surface-at-friction footers for the exec/shell
// deny/refuse families. It implements Fix 1 of the signed-off capability-
// discovery audit (researches/decisions/2026-08-04-capability-discovery-audit.md
// §1 and §9): when an agent hits a denial/refusal, the message must
//
//	{preserve the denial} → {explain why} → {name the sanctioned alternative}
//	→ {point to the authority/rule} → {never auto-retry}.
//
// The exec-ro DENY ladder (internal/execro/classifier.go denyFooter) is the
// POSITIVE CONTROL and the reference shape for this principle. These footers
// bring the exec/shell gate sites in internal/cli/exec_shell.go to the same
// shape WITHOUT touching exec-ro, WITHOUT introducing any new verb/command
// (the advisory-verb idea was rejected), and WITHOUT changing what is denied —
// only how the denial is explained.
//
// Why local footers instead of extracting exec-ro's denyFooter into a shared
// helper: exec-ro's footer is tightly coupled to the read-only execution ladder
// (exec-ro → exec-sandbox → bare, with runshape floor binding via
// runshape.FindMinMode). The exec/shell gate sites need a DIFFERENT authority
// pointer (the shell-guard plugin rules that carry each forbidden pattern's own
// `why` + AGENTS.md shell hygiene), not the read-only ladder. Forcing both into
// one shared helper would either lose exec-ro's load-bearing specificity or
// couple the generic exec-deny path to runshape concerns. Three distinct
// friction shapes (exec/shell safety-deny, capability-boundary) get local
// footers; this is the smallest blast radius and keeps exec-ro stable.
//
// The footers are rendered to cmd.ErrOrStderr() alongside the gate reason (the
// reason itself already explains WHY). The returned error stays a concise typed
// signal (cobra's "Error:" line is secondary); it does not carry the footer.

// execDenyFooter is appended to every exec/shell permission-gate DENY. The gate
// reason (carried by the caller) explains why this specific command was denied;
// this footer adds the never-auto-retry directive and points at the canonical
// authority that names the sanctioned alternative for the matched rule.
//
// The authority pointer is the shell-guard forbidden-patterns rules: each rule
// carries a `why` field (and several carry an inline `alternative`) — see
// templates/core/.opencode/repo-configs/forbidden-patterns.core.js. AGENTS.md →
// "Shell, container, and workspace hygiene" restates the operator contract and
// the "read the rule's why and pick the canonical alternative" discipline.
// (docs/ai/shell-execution.md is referenced from AGENTS.md but is currently a
// dangling reference; the plugin + AGENTS.md are the real friction-time
// authorities, so the footer points there.)
func execDenyFooter() string {
	return "\n" +
		"- This denial is final: do not retry the command, paraphrase it, or route it " +
		"through another verb or agent to get around the gate.\n" +
		"- For the sanctioned alternative, read the matching shell-guard rule's `why` " +
		"(.opencode/repo-configs/forbidden-patterns*.js) and AGENTS.md → " +
		"\"Shell, container, and workspace hygiene\"."
}

// hookErrorFooter is appended when the permission hook itself faulted. The gate
// is fail-closed (any bridge fault → Deny by default for safety), so the message
// must say so plainly, forbid retry/bypass, and point at the diagnosis path
// rather than at a rule's `why` (no rule matched — the engine broke).
func hookErrorFooter() string {
	return "\n" +
		"- The permission gate faulted and denied by default for safety; do not retry or bypass it.\n" +
		"- Diagnose the shell-guard bridge with `vh-agent-harness doctor` (node availability, " +
		"eval.js at .opencode/plugins/shell-guard/). See AGENTS.md → " +
		"\"Shell, container, and workspace hygiene\"."
}

// askFooter is appended when the gate returned Ask (undecided) and no
// operator-confirmation loop is attached. The CLI treats this as deny-by-default;
// the footer forbids auto-retry and names the two real resolutions (surface to
// the operator, or use an interactive shell where the operator can confirm).
func askFooter() string {
	return "\n" +
		"- The gate could not decide and no operator-confirmation loop is attached, so this is " +
		"denied by default; do not auto-retry.\n" +
		"- Surface the decision to the operator, or use an interactive `vh-agent-harness shell` " +
		"where the operator can confirm."
}
