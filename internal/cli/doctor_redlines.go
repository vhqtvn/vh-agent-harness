package cli

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/vhqtvn/vh-agent-harness/internal/redlines"
)

// redlinesUserRegistryPresent reports whether a user-level redlines registry
// file exists at the resolved XDG path. It is the APPLICABILITY gate for the
// redlines doctor section: when false, runDoctor emits NO redlines section at
// all (zero footprint for non-adopters — no warning, no output, no file). This
// mirrors the dev-stale-embed #15 conditional-section precedent.
//
// Presence is tested via os.Stat (NOT Load) so an INVALID-but-present registry
// still triggers the section (the loadability sub-check reports the invalid
// state). Path-resolution failure is treated as non-applicable (the section
// cannot meaningfully run without a resolvable config base).
func redlinesUserRegistryPresent() bool {
	path, err := redlines.UserRegistryPath()
	if err != nil {
		return false
	}
	return isRegularFile(path)
}

// checkPrivateRedlines is the redlines doctor hygiene check. It is APPLICABILITY-
// GATED: when no user-level registry exists it is never invoked (runDoctor omits
// the section entirely via redlinesUserRegistryPresent). When a registry IS
// present it runs four paste-safe sub-checks:
//
//  1. FILE SECURITY (WARN): user-level + repo-local registry files must not be
//     group/world-readable. POSIX-only (CheckFileSecurity no-ops elsewhere).
//  2. TRACKED-DESPITE-SENSITIVE (FAIL): the repo-local additive registry
//     (.vh-agent-harness/redlines.local.yml) holds private terms; if it is
//     tracked by git (or present and NOT gitignored) that is an active leak.
//     Mirrors checkAutoGateGitignored's tracked/present+unignored discipline.
//  3. LOADABILITY (WARN): Load must succeed. A present-but-invalid registry
//     fails closed in the commit gate on every commit; doctor surfaces it as a
//     WARN with an OPAQUE error (Load errors never echo terms/labels/sides/why).
//  4. BINDING HYGIENE (paste-safe detail): when Load succeeds, reports how many
//     subjects bind this repository, listing OPAQUE subj-* IDs only. NEVER
//     labels/sides/why — those are guidance-only.
//
// All diagnostics are paste-safe: the only identifiers emitted are opaque
// subject IDs, registry file paths (which contain no terms), generic reason
// codes, and raw permission bits. The full-term surface is `redlines guidance`
// alone (unchanged).
func checkPrivateRedlines(target string) checkResult {
	const name = "redlines"

	// Defensive applicability re-check (runDoctor already gates on
	// redlinesUserRegistryPresent; this makes the function safe to call
	// directly without leaking a non-applicable PASS).
	if !redlinesUserRegistryPresent() {
		return checkResult{name: name, tier: tierSkip,
			detail: "no user registry (non-applicable)"}
	}

	var fails, warns []string

	// Sub-check 1: user-level registry file security (mode exposure).
	userPath, _ := redlines.UserRegistryPath()
	if userPath != "" {
		if sec := redlines.CheckFileSecurity(userPath); sec.Checked && sec.GroupOrWorldReadable {
			warns = append(warns, fmt.Sprintf(
				"user registry %s: mode %04o is group/world-readable — tighten to 0600 (chmod 0600 %s)",
				redlinesSafePath(userPath), sec.Mode, redlinesSafePath(userPath)))
		}
	}

	// Sub-check 2: repo-local additive registry — security + tracked/ignored.
	repoLocalRel := filepath.ToSlash(filepath.Join(".vh-agent-harness", "redlines.local.yml"))
	repoLocalAbs := filepath.Join(target, filepath.FromSlash(repoLocalRel))
	if isRegularFile(repoLocalAbs) {
		if rsec := redlines.CheckFileSecurity(repoLocalAbs); rsec.Checked && rsec.GroupOrWorldReadable {
			warns = append(warns, fmt.Sprintf(
				"%s: mode %04o is group/world-readable — tighten to 0600",
				repoLocalRel, rsec.Mode))
		}
		// tracked / ignored probes need git + a work tree (mirror
		// checkAutoGateGitignored's availability guard).
		if _, err := exec.LookPath("git"); err == nil {
			if wt, err := exec.Command("git", "-C", target, "rev-parse", "--is-inside-work-tree").Output(); err == nil &&
				strings.TrimSpace(string(wt)) == "true" {
				tracked, indet := gitTracked(target, repoLocalRel)
				if !indet {
					switch {
					case tracked:
						fails = append(fails, fmt.Sprintf(
							"%s: TRACKED by git — a sensitive registry is in the index/history; "+
								"remove from the index (git rm --cached %s) and scrub history if exposed",
							repoLocalRel, repoLocalRel))
					default:
						ignored, src, indet2 := gitCheckIgnoreVerbose(target, repoLocalRel)
						if !indet2 {
							if !ignored {
								fails = append(fails, fmt.Sprintf(
									"%s: present and NOT gitignored — would be staged on the next git add; add a .gitignore rule",
									repoLocalRel))
							} else if !portableIgnoreSource(src) {
								warns = append(warns, fmt.Sprintf(
									"%s: ignored only via non-portable source (%s) — add a repo .gitignore rule so the protection is shared",
									repoLocalRel, src))
							}
						}
					}
				}
			}
		}
	}

	// Sub-check 3 + 4: loadability, then binding hygiene (only when loadable).
	bindingSummary := ""
	reg, loadErr := redlines.Load(target)
	switch {
	case loadErr != nil:
		// OPAQUE error (Load never echoes terms). oneLine keeps the detail flat.
		warns = append(warns, fmt.Sprintf(
			"registry present but unreadable/invalid: %s", oneLine(loadErr.Error())))
	case reg != nil:
		remotes, _ := redlines.RepoRemotes(target)
		var binding []string
		for _, s := range reg.Subjects {
			if s.Binds(target, remotes) {
				binding = append(binding, s.ID) // ID is the only safe-to-echo field
			}
		}
		sort.Strings(binding)
		switch len(binding) {
		case 0:
			bindingSummary = "0 subjects bind this repository (registry present but inert here)"
		default:
			bindingSummary = fmt.Sprintf("%d subject(s) bind this repository: %s",
				len(binding), strings.Join(binding, ", "))
		}
	default:
		// (nil, nil): registry absent at load time (TOCTOU vs the presence
		// gate). Nothing to report; treat as inert.
		bindingSummary = "user registry vanished between detection and load (inert)"
	}

	// Aggregate: FAIL > WARN > PASS, with the binding summary appended to detail.
	detail := bindingSummary
	if len(fails) > 0 || len(warns) > 0 {
		parts := append([]string{}, fails...)
		parts = append(parts, warns...)
		if detail != "" {
			parts = append(parts, detail)
		}
		detail = strings.Join(parts, "; ")
	}
	switch {
	case len(fails) > 0:
		return checkResult{name: name, tier: tierFail, detail: detail}
	case len(warns) > 0:
		return checkResult{name: name, tier: tierWarn, detail: detail}
	default:
		return checkResult{name: name, tier: tierPass, detail: detail}
	}
}

// redlinesSafePath reduces a registry file path to its base name for diagnostic
// output, avoiding repeating a long absolute XDG path. The path itself contains
// no sensitive terms (it is .../vh-agent-harness/redlines/registry.yml), but the
// short form keeps the detail line readable. The full path is only used when
// the operator needs the chmod target.
func redlinesSafePath(path string) string {
	if base := filepath.Base(path); base != "" {
		return base
	}
	return path
}
