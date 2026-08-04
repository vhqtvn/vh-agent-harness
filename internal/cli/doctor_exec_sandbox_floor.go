package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/vhqtvn/vh-agent-harness/internal/permconfig"
	"github.com/vhqtvn/vh-agent-harness/internal/runshape"
)

// checkExecSandboxGrantFloor is the FIX-3 advisory doctor check. It surfaces the
// residual blast-radius of the exec-sandbox Level-B grant when NO
// exec_sandbox.min_mode floor resolves for the repo: an agent that carries the
// grant (researcher / repo-explorer / media-perception — the rendered
// `vh-agent-harness exec-sandbox *: allow` in opencode.jsonc) is one explicit
// `--sandbox=off` downgrade away from running fully uncontained in an unfloored
// repo.
//
// The grant ships to consumers via the core corpus, but run-shape.yml is
// PROJECT-OWNED — the harness cannot ship a floor with the grant. Fix 2 seeds
// `min_mode: strict` for NEW installs, but existing adopters keep an
// already-seeded run-shape without the block. Fix 1 (the binary-side refuse on
// absent + explicit --sandbox=off) is what actually closes those existing
// adopters; THIS check is the advisory that surfaces the unfloored state so an
// operator can add `exec_sandbox.min_mode: strict` (the durable close) rather
// than relying solely on the refuse.
//
// TIERING — ADVISORY ONLY, NEVER FAIL:
//   - grant carried AND no floor resolves -> WARN (the residual risk Fix 1
//     refuses at runtime but the operator has not yet durably floored the repo).
//   - grant carried AND a floor resolves (any value, including explicit off) ->
//     PASS (the repo has a deliberate floor posture: contained, or a conscious
//     opt-out).
//   - no grant carried (no agent has the exec-sandbox allow) -> SKIP (the
//     advisory is moot — no agent can invoke exec-sandbox).
//   - opencode.jsonc absent or unparseable -> SKIP (config-refs / managed-drift
//     own that surface; do not double-report).
//   - FindMinMode errors (present-but-broken floor) -> SKIP (the runtime
//     fail-closed already refuses uncontained on a broken floor; doctor's
//     advisory here is only about the absent-floor case, and a broken floor is
//     not "no floor"). The armed-schema / fail-closed surfaces own broken floors.
//
// This check NEVER increments the problem count: it is the SAFETY LAYER INFORMing
// (doctor detects + reports; the binary-side refuse in applyFloorToRequest ACTs).
func checkExecSandboxGrantFloor(target string) checkResult {
	const name = "exec-sandbox-floor"

	// 1. Read the live project opencode.jsonc (the rendered grant surface).
	//    Absent/unparseable -> SKIP (other checks own that surface).
	ocPath := filepath.Join(target, "opencode.jsonc")
	raw, err := os.ReadFile(ocPath)
	if err != nil {
		// Try the .json variant too (OpenCode accepts both); if neither is
		// present this is not an installed seam target for this check.
		ocPathJSON := filepath.Join(target, "opencode.json")
		raw, err = os.ReadFile(ocPathJSON)
		if err != nil {
			return checkResult{name: name, tier: tierSkip,
				detail: "no opencode.json[c] (not installed or core-only); grant surface absent"}
		}
	}
	doc, ok := parseOpencodeConfigDoc(raw)
	if !ok {
		return checkResult{name: name, tier: tierSkip,
			detail: "opencode.jsonc unparseable (config-refs/managed-drift own that surface)"}
	}

	// 2. Does any agent carry the exec-sandbox allow grant? Walk agent.<*>.
	//    permission.bash[ExecSandboxCommand] == "allow". The grant is the
	//    rendered form of permconfig's ReadOnlyExtraAllows emission.
	granted := agentsCarryingExecSandboxGrant(doc)
	if len(granted) == 0 {
		return checkResult{name: name, tier: tierSkip,
			detail: "no agent carries the exec-sandbox grant (Level-B mechanism absent)"}
	}

	// 3. Does a floor resolve? FindMinMode walks the whole ancestor chain;
	//    floorRaw == "" means NO exec_sandbox.min_mode block exists anywhere
	//    (the absent case Fix 1 refuses). Any non-empty floorRaw
	//    (off|best-effort|strict, including an explicit deliberate opt-out)
	//    counts as "a floor resolves".
	_, floorRaw, ferr := runshape.FindMinMode(target)
	if ferr != nil {
		// A present-but-BROKEN floor is a different concern (the runtime
		// fail-closed refuses uncontained on a broken floor). This advisory is
		// only about the absent case, so SKIP rather than warn on a state the
		// fail-closed surfaces already own.
		return checkResult{name: name, tier: tierSkip,
			detail: "floor resolution errored (broken floor; runtime fail-closed owns this surface)"}
	}
	if floorRaw == "" {
		// WARN (advisory only — never FAIL): the grant is carried but no floor
		// resolves. Fix 1 refuses an explicit --sandbox=off at runtime, but the
		// operator has not durably floored the repo. Surface the residual + the
		// durable fix.
		sort.Strings(granted)
		return checkResult{name: name, tier: tierWarn,
			detail: fmt.Sprintf(
				"exec-sandbox Level-B grant carried by %d agent(s) [%s] but no exec_sandbox.min_mode floor resolves in any enclosing run-shape.yml — "+
					"an explicit --sandbox=off downgrade is refused at runtime (Fix 1), but the repo is not durably floored. "+
					"Add `exec_sandbox: { min_mode: strict }` under .vh-agent-harness/run-shape.yml to durably contain the grant.",
				len(granted), strings.Join(granted, ", "))}
	}
	return checkResult{name: name, tier: tierPass,
		detail: fmt.Sprintf("exec-sandbox grant carried by %d agent(s); floor resolves (min_mode=%q)", len(granted), floorRaw)}
}

// agentsCarryingExecSandboxGrant walks the agent map in a parsed opencode config
// and returns the sorted list of agent names whose permission.bash block carries
// the exec-sandbox allow entry. It is defensive against shape variation (a
// missing permission/bash block, non-object agents, etc.) — any structural
// deviation simply means that agent does not carry the grant.
func agentsCarryingExecSandboxGrant(doc map[string]any) []string {
	if doc == nil {
		return nil
	}
	agentMap, ok := doc["agent"].(map[string]any)
	if !ok {
		return nil
	}
	var carried []string
	for agentName, v := range agentMap {
		agent, ok := v.(map[string]any)
		if !ok {
			continue
		}
		perm, ok := agent["permission"].(map[string]any)
		if !ok {
			continue
		}
		bash, ok := perm["bash"].(map[string]any)
		if !ok {
			continue
		}
		// The rendered grant key is permconfig.ExecSandboxCommand. An agent
		// "carries" the grant when its bash block resolves the verb to allow.
		if val, exists := bash[permconfig.ExecSandboxCommand]; exists && val == "allow" {
			carried = append(carried, agentName)
		}
	}
	return carried
}
