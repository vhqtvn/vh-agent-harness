package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/vhqtvn/vh-agent-harness/internal/complexity"
)

// checkComplexityAdvisory is doctor named check #20 (staged advisory hybrid).
// It loads the platform_armed complexity-policy.yml from the target tree, runs
// the repo-snapshot scanner, and surfaces nominated candidates (observed >
// configured threshold) as a WARNING-ONLY advisory.
//
// SACRED INVARIANT (warning-only): this check is structurally incapable of
// returning tierFail. It returns:
//   - tierSkip  when the policy file is absent (greenfield) or disabled
//     (enabled: false);
//   - tierPass  when the policy is active but no file exceeds its threshold;
//   - tierWarn  when one or more files exceed their threshold (advisory
//     surfaced — the operator should record a disposition).
//
// It NEVER increments the problem count and NEVER authorizes a transition. A
// complexity threshold breach is advisory evidence, not authority. This is the
// authority line: the SAFETY LAYER INFORMs (doctor WARNs; it does not block).
//
// The policy file is read via os.ReadFile (not the embedded corpus) so the check
// reflects the LIVE project instance, including any project overrides within the
// schema envelope. A malformed policy is surfaced as a WARN (the armed-schema
// check #2 is the authoritative validator and will FAIL on it); this check never
// escalates a schema problem to a FAIL.
func checkComplexityAdvisory(target string) checkResult {
	const name = "complexity-advisory"
	policyPath := filepath.Join(target, ".vh-agent-harness", "complexity-policy.yml")
	raw, err := os.ReadFile(policyPath)
	if err != nil {
		if os.IsNotExist(err) {
			return checkResult{name: name, tier: tierSkip,
				detail: "complexity-policy.yml absent (greenfield or not installed)"}
		}
		// An unreadable policy is a WARN here (not FAIL); armed-schema owns the
		// authoritative lint. Surface the read problem without escalating.
		return checkResult{name: name, tier: tierWarn,
			detail: fmt.Sprintf("read complexity-policy.yml: %v", err)}
	}
	policy, perr := complexity.LoadPolicy(raw)
	if perr != nil {
		// A malformed policy is a WARN here (armed-schema #2 FAILs on it).
		return checkResult{name: name, tier: tierWarn,
			detail: fmt.Sprintf("parse complexity-policy.yml: %v (see armed-schema for the authoritative lint)", perr)}
	}
	if !policy.Enabled {
		return checkResult{name: name, tier: tierSkip,
			detail: "complexity policy disabled (enabled: false); no advisory"}
	}
	signals, serr := complexity.ScanRepo(target, policy)
	if serr != nil {
		// A scanner error is non-fatal: surface as WARN (advisory could not run).
		return checkResult{name: name, tier: tierWarn,
			detail: fmt.Sprintf("scan: %v", serr)}
	}
	nominated := complexity.Nominated(signals)
	if len(nominated) == 0 {
		return checkResult{name: name, tier: tierPass,
			detail: "no files exceed the configured complexity thresholds"}
	}
	maxCandidates := policy.Doctor.MaxCandidates
	if maxCandidates <= 0 {
		maxCandidates = 10
	}
	section := complexity.DoctorSection(nominated, maxCandidates, true)
	return checkResult{name: name, tier: tierWarn,
		detail: strings.TrimSpace(section)}
}
