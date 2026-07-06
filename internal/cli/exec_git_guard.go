package cli

import (
	"github.com/vhqtvn/vh-agent-harness/internal/execro"
	"github.com/vhqtvn/vh-agent-harness/internal/permconfig"
)

// denyExecGitMutationPayload reports whether the bare `exec` payload args are a
// git mutation that must route through the commit-gate. It is the A1 Go binary
// backstop for the F1 bypass: `vh-agent-harness exec git <global-flag>
// <mutation>` (e.g. `exec git --no-pager commit`, `exec git -C /x push`,
// `exec git --git-dir=/x commit`) currently reaches evaluateGate's harness
// branch where the JS adjacency regex (`git-mutation-bypass`) cannot match a
// flag between `git` and the verb, so the mutation slips past the commit-gate.
// This guard denies it at the Go binary BEFORE the JS bridge even runs
// (defense in depth alongside the A2 source-of-truth fix in
// shell-guard-core.js's harness branch).
//
// It is git-mutation-scoped ONLY:
//
//   - It does NOT default-deny. Regular `exec` is SUPPOSED to allow arbitrary
//     non-git mutations (mkdir, pytest, npm, `bash -c '...'`); calling
//     execro.Classify here (curated-allowlist / default-deny) would break that
//     legitimate surface. This is why A1 cannot reuse execro.Classify.
//   - It does NOT parse nested shell strings (`exec bash -c 'git …'` is
//     explicitly out of scope — that stays governed by the existing
//     forbidden-pattern scan in the JS gate).
//   - It does NOT replicate exec-ro's hard-deny of config/exec-affecting
//     globals (-c/--config-env/--exec-path); those are consume-and-continue
//     here (via execro.GitVerbPastGlobals) because the only goal is to reach
//     the verb. A mutation hidden behind any global flag is still caught.
//
// args is the BARE exec payload (e.g. ["git","--no-pager","commit"]), NOT
// prefixed with `vh-agent-harness exec`. Returns (true, reason) when
// args[0]=="git" and the first git verb past global flags is in
// permconfig.GitMutationVerbsSet; (false, "") otherwise (read-only git verbs,
// non-git payloads, empty args).
func denyExecGitMutationPayload(args []string) (bool, string) {
	if len(args) == 0 || args[0] != "git" {
		return false, ""
	}
	verb := execro.GitVerbPastGlobals(args)
	if verb == "" {
		return false, ""
	}
	if permconfig.GitMutationVerbsSet[verb] {
		return true, "Git mutations must go through the commit-gate wrapper. " +
			"Only the committer agent (C) may execute git writes, and only " +
			"through `.opencode/scripts/commit-gate.sh`. " +
			"See .opencode/docs/git-execution-routing.md. " +
			"(Wrapped `vh-agent-harness exec git …` routed verb '" + verb +
			"' past a global flag.)"
	}
	return false, ""
}
