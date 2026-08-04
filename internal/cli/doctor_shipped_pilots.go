package cli

// doctor_shipped_pilots.go implements doctor check #23 (shipped-pilots). It
// surfaces the enablement state of each shipped default-on overlay pilot — the
// 3 skills-only INFORMS-only packs activated by the features: map — so an
// operator can see at a glance whether a pilot is enabled-by-platform-default,
// disabled-by-consumer-override, or explicitly-selected via overlays:.
//
// GATE-2 ADVISORY ORPHAN — NEVER FAIL: opting out of a pilot (features:
// <key>: false) stops future rendering/staging but does NOT auto-delete the
// previously-rendered skill files. A skill file left on disk from a now-
// deselected pilot is an ADVISORY orphan: surfaced here as tierInfo for
// operator visibility, but it NEVER increments the problem count and NEVER
// makes the repo unhealthy. This is consistent with the render path (pack
// deselection is not an orphan finding because the source still exists in the
// embed) and with doctor's managed-drift check (which only covers
// platform_managed paths; overlay_extension pilot paths are outside its scope).
// This check provides the enablement visibility those surfaces cannot.

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/vhqtvn/vh-agent-harness/internal/overlay"
)

// pilotStateEnabledDefault is the state label for a pilot whose feature key
// resolves true via the platform-default reconciliation (the consumer did not
// explicitly opt out, and the pack is not listed under overlays:).
const pilotStateEnabledDefault = "enabled-by-platform-default"

// pilotStateDisabledOverride is the state label for a pilot the consumer
// explicitly disabled via features: <key>: false (and the pack is not listed
// under overlays: — an explicit overlays: entry survives the false).
const pilotStateDisabledOverride = "disabled-by-consumer-override"

// pilotStateExplicit is the state label for a pilot listed under overlays:
// in the live profile. An explicit overlays: entry survives a features: false
// (it is NOT a global veto), so this state takes precedence over the feature
// value.
const pilotStateExplicit = "explicitly-selected"

// checkShippedPilots is doctor check #23. It reports the enablement state of
// each shipped default-on overlay pilot and surfaces advisory orphans (skill
// files left on disk from a now-deselected pilot). The check is ADVISORY ONLY:
// it never increments the problem count and never makes the repo unhealthy.
//
// TIERING:
//   - all pilots active, no stale dirs → tierPass
//   - any pilot disabled (consumer choice) → tierPass (opt-out is intentional)
//   - any advisory orphan (disabled pilot with stale skill files on disk) →
//     tierInfo (surfaces the stale files for manual cleanup; NOT a failure)
//   - not seam-installed (greenfield/adoptable) → tierSkip
func checkShippedPilots(target string) checkResult {
	const name = "shipped-pilots"

	// SKIP when the harness is not installed here. The feature defaults are
	// meaningful only in an installed repo; in a greenfield/adoptable repo the
	// pilot enablement is moot (nothing is rendered yet).
	if !isSeamInstalled(target) {
		return checkResult{name: name, tier: tierSkip,
			detail: "not seam-installed (greenfield or adoptable)"}
	}

	features := reconciledFeatures(target)
	explicitOverlays := activeOverlays(target)

	// Deterministic iteration order: sorted feature keys.
	featureKeys := make([]string, 0, len(featurePackMap))
	for k := range featurePackMap {
		featureKeys = append(featureKeys, k)
	}
	sort.Strings(featureKeys)

	type pilotInfo struct {
		packName    string
		state       string
		orphanCount int
	}

	var infos []pilotInfo
	orphanTotal := 0

	for _, fk := range featureKeys {
		packName := featurePackMap[fk]
		isExplicit := false
		for _, o := range explicitOverlays {
			if o == packName {
				isExplicit = true
				break
			}
		}
		isFeatureTrue := features[fk]

		var state string
		isActive := false
		switch {
		case isExplicit:
			state = pilotStateExplicit
			isActive = true
		case isFeatureTrue:
			state = pilotStateEnabledDefault
			isActive = true
		default:
			state = pilotStateDisabledOverride
			isActive = false
		}

		orphanCount := 0
		if !isActive {
			// Advisory orphan: check whether this pilot's skill files remain
			// on disk from a prior render. These are NOT failures — they are
			// surfaced for manual cleanup visibility.
			orphanCount = countPilotOrphanFiles(target, packName)
			orphanTotal += orphanCount
		}

		infos = append(infos, pilotInfo{
			packName:    packName,
			state:       state,
			orphanCount: orphanCount,
		})
	}

	var parts []string
	for _, info := range infos {
		part := fmt.Sprintf("%s: %s", info.packName, info.state)
		if info.orphanCount > 0 {
			part += fmt.Sprintf(" (advisory orphan: %d stale file(s) on disk; remove manually if undesired)", info.orphanCount)
		}
		parts = append(parts, part)
	}
	detail := strings.Join(parts, "; ")

	if orphanTotal > 0 {
		return checkResult{name: name, tier: tierInfo, detail: detail}
	}
	return checkResult{name: name, tier: tierPass, detail: detail}
}

// countPilotOrphanFiles checks whether a deselected pilot's rendered skill
// files remain on disk. It opens the pack from the embedded corpus, enumerates
// the unit paths it WOULD render (the same set RenderUnits writes), and counts
// how many of those paths exist in the live .opencode/ tree. A non-zero count
// means the pilot was previously rendered and its output was not manually
// removed after opt-out — an advisory orphan, never a failure.
func countPilotOrphanFiles(target, packName string) int {
	pack, err := overlay.OpenPack(packName)
	if err != nil {
		// Pack unreadable from the embed: cannot determine expected paths.
		// Do not fabricate orphan findings; return 0.
		return 0
	}
	unitPaths, err := pack.UnitPaths()
	if err != nil {
		return 0
	}
	count := 0
	for _, rel := range unitPaths {
		livePath := filepath.Join(target, filepath.FromSlash(rel))
		if _, err := os.Stat(livePath); err == nil {
			count++
		}
	}
	return count
}
