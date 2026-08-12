package cli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
)

// checkAgentsComposition is the agents-composition doctor check (WARN-only
// observability, NEVER FAIL). It validates that the live root AGENTS.md equals
// the body the seam WOULD compose from the project's sources
// (.vh-agent-harness/AGENTS.core.md + AGENTS.mission.md), so a stale or
// hand-edited unified AGENTS.md is surfaced without disrupting the repo.
//
// OPT-IN, like the seam composition itself:
//
//   - .vh-agent-harness/AGENTS.mission.md ABSENT → SKIP. A legacy consumer that
//     never adopted the core/mission split has a legitimately hand-authored root
//     AGENTS.md (project_owned); the seam never composes it, so doctor must not
//     flag it. This mirrors composeAgentsMd's no-op-on-no-mission rule.
//   - mission source present → compose the expected body via the SAME helper
//     composeAgentsMd uses (composeAgentsMdBytes), then byte-compare against the
//     live root AGENTS.md. WARN (NOT FAIL) on any difference (drifted content,
//     missing root file). WARN-only by construction: this is read-only
//     observability over a project_owned file — it must carry zero disruption
//     risk and never make the repo UNHEALTHY. Re-running `vh-agent-harness
//     update` re-composes the root file and clears the WARN.
//
// The root AGENTS.md ownership classification is UNCHANGED (stays project_owned);
// this check observes only. It does NOT change the carve-out semantics of any
// other check.
func checkAgentsComposition(target string) checkResult {
	const name = "agents-composition"

	expected, present, err := composeAgentsMdBytes(target)
	if err != nil {
		// A read/template error is outside this check's contract (the
		// managed-drift / armed-schema surfaces own source-file health); SKIP
		// rather than double-report. The WARN path is reserved for a present
		// but stale composed body, which is the actionable signal.
		return checkResult{name: name, tier: tierSkip,
			detail: fmt.Sprintf("compose skipped: %v", err)}
	}
	if !present {
		return checkResult{name: name, tier: tierSkip,
			detail: "no .vh-agent-harness/AGENTS.mission.md (legacy consumer; unified AGENTS.md is legitimately hand-edited)"}
	}

	live, rerr := os.ReadFile(filepath.Join(target, "AGENTS.md"))
	if rerr != nil {
		if os.IsNotExist(rerr) {
			return checkResult{name: name, tier: tierWarn,
				detail: "mission source present but root AGENTS.md is absent — run `vh-agent-harness update` to compose it"}
		}
		return checkResult{name: name, tier: tierWarn,
			detail: fmt.Sprintf("read root AGENTS.md: %v", rerr)}
	}
	if !bytes.Equal(live, expected) {
		return checkResult{name: name, tier: tierWarn,
			detail: "root AGENTS.md differs from the composed AGENTS.core.md + AGENTS.mission.md body — run `vh-agent-harness update` to re-compose"}
	}
	return checkResult{name: name, tier: tierPass,
		detail: "root AGENTS.md matches the composed AGENTS.core.md + AGENTS.mission.md body"}
}
