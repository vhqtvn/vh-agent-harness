package cli

// overlay_list.go implements `vh-agent-harness overlay list` — the discovery
// command that enumerates EVERY pack visible to the project (embedded +
// project-local) with its source and selected status, so a coordinator or
// operator learns a pack EXISTS before authorizing a rebuild-from-scratch.
//
// It exists to kill the false-negative recorded in the 2026-08-06 incident:
// a coordinator concluded `auto-classifier-pilot` "does not exist" while it
// ships embedded, because every overlay-capability enumeration surface at the
// time projected ONLY already-selected (active) packs. `overlay list` projects
// the full shipped set instead, so an unselected-but-shipped pack surfaces as
// "available", never as absent.
//
// The pack list comes from overlay.KnownPacksFor (embedded UNION project-local
// with source attribution) — NOT from rendered-outputs.json, which carries
// provenance only for already-selected packs and therefore cannot enumerate
// unselected ones. Selection status is MEMBERSHIP in the canonical render
// pack-set (renderPackSet -> resolveCapabilityAnswers output, the same closure
// the render seam consumes), so a pack pulled in TRANSITIVELY via another
// selected pack's hard_dep is reported selected. The render-set read is
// best-effort: a resolver/profile error degrades to the legacy direct signals
// (overlays / capabilities / feature pilots) so `overlay list` stays non-fatal.

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/vhqtvn/vh-agent-harness/internal/overlay"
	"github.com/vhqtvn/vh-agent-harness/internal/resolver"
	"github.com/vhqtvn/vh-agent-harness/internal/runshape"
)

// overlayListFl holds the inputs to `vh-agent-harness overlay list`.
type overlayListFlags struct {
	target string
}

var overlayListFl *overlayListFlags

var overlayListCmd = &cobra.Command{
	Use:           "list",
	Short:         "List every pack (embedded + project-local) with source + selected status",
	SilenceUsage:  true,
	SilenceErrors: true,
	Long: `List every overlay pack visible to this project, one line per pack, with
its source (embedded in the binary vs. project-local under
.vh-agent-harness/overlays/) and its selected status (renders on the next
` + "`update`" + ` vs. available-but-unselected).

The list projects the FULL shipped set, not just already-selected packs: an
unselected-but-shipped pack (e.g. ` + "`auto-classifier-pilot`" + `, ` + "`release`" + `,
` + "`repo-mail`" + `) surfaces as "available", never as absent. This is the
discovery surface that prevents a coordinator from wrongly concluding a
shipped pack does not exist.

A pack is "selected" when it will RENDER on the next ` + "`update`" + ` — the SAME
pack-set the render seam computes, so a pack pulled in TRANSITIVELY via another
selected pack's hard_dep (e.g. ` + "`release`" + ` renders when a selected pack declares
` + "`hard_deps: [core/release]`" + `) is selected, not merely available. A pack reaches
the render set when ANY of:
  - its name is listed under ` + "`overlays:`" + ` in vh-harness-profile.yml, OR
  - its capability-manifest id is listed under ` + "`capabilities:`" + ` (the
    ` + "`release`" + ` pack is dual-selectable via ` + "`core/release`" + `), OR
  - it is a shipped default-on pilot whose feature key resolves true, OR
  - another selected pack's capability hard-deps on it (transitive closure).

Run ` + "`vh-agent-harness overlay docs <name>`" + ` to read a pack's README and
learn how to configure it before enabling.`,
	Args: cobra.NoArgs,
	RunE: runOverlayList,
}

func init() {
	overlayListFl = &overlayListFlags{target: "."}
	overlayListCmd.Flags().StringVarP(&overlayListFl.target, "target", "o", overlayListFl.target,
		"project root containing .vh-agent-harness/ (default: current directory)")

	overlayCmd.AddCommand(overlayListCmd)
}

// runOverlayList is the RunE for `vh-agent-harness overlay list`.
func runOverlayList(cmd *cobra.Command, _ []string) error {
	out := cmd.OutOrStdout()

	projectRoot := resolveOverlayTarget(overlayListFl.target)

	packs, err := overlay.KnownPacksFor(projectRoot)
	if err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "error: enumerate overlay packs: %v\n", err)
		return errSilent{}
	}

	selection := resolvePackSelection(projectRoot, packs)

	// Render the table with text/tabwriter so columns align and each pack line
	// stays grep-friendly (name, source, and status all on one line).
	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "PACK\tSOURCE\tSTATUS")
	for _, ep := range selection {
		status := "available"
		if ep.Selected {
			status = "selected"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\n", ep.Name, ep.Source, status)
	}
	if err := tw.Flush(); err != nil {
		return err
	}

	fmt.Fprintln(out)
	fmt.Fprintln(out, "Selected = renders on the next `vh-agent-harness update`; available = shipped/visible but not selected.")
	fmt.Fprintln(out, "Read a pack's docs: `vh-agent-harness overlay docs <name>`.")
	fmt.Fprintln(out, "Enable a pack: add it under `overlays:` in .vh-agent-harness/vh-harness-profile.yml, then `vh-agent-harness update`.")
	return nil
}

// enrichedPack is one enumerated pack with its resolved source + selected
// status for `overlay list` (and the guide hint).
type enrichedPack struct {
	overlay.PackInfo
	Selected bool
}

// packSelectionState carries the once-computed selection state shared by
// resolvePackSelection (`overlay list`) and unselectedEmbeddedPacks (guide's
// ungated hint): the canonical render pack-set when the resolver succeeded,
// PLUS the legacy direct signals used as a fallback when it did not. Computing
// both once consolidates the near-duplicate profile reads the dropped F2 review
// flagged (each function re-read the profile twice through activeOverlays /
// parseProfileSelection / reconciledFeatures).
type packSelectionState struct {
	// renderSet is the canonical render pack-set (resolveCapabilityAnswers
	// output via renderPackSet) as a membership map. nil when renderOK is false.
	renderSet map[string]bool
	renderOK  bool

	// Legacy fallback signals (always best-effort populated): used when
	// renderOK is false so `overlay list` stays non-fatal on a broken profile.
	overlays     []string
	capabilities []resolver.CapabilityID
	features     map[string]bool
}

// loadPackSelection reads the selection state for target ONCE. Never fails: a
// resolver/catalog/profile-read error sets renderOK=false and the legacy signals
// (themselves best-effort) drive the answer, so `overlay list` stays non-fatal
// on a broken profile (every pack surfaces as available in the worst case).
func loadPackSelection(target string) packSelectionState {
	raw, rerr := os.ReadFile(filepath.Join(target, harnessProfileName))
	if rerr != nil {
		raw = nil
	}
	_, capabilities := parseProfileSelection(raw)
	st := packSelectionState{
		overlays:     activeOverlays(target),
		capabilities: capabilities,
		features:     reconciledFeatures(target),
	}
	packs, ok := renderPackSet(target)
	if ok {
		st.renderOK = true
		st.renderSet = make(map[string]bool, len(packs))
		for _, n := range packs {
			st.renderSet[n] = true
		}
	}
	return st
}

// isSelected reports whether packName would render on the next update for the
// project at target, per st. Selection is MEMBERSHIP in the canonical render
// pack-set (resolveCapabilityAnswers output — the SAME closure the render seam
// consumes), so a pack pulled in TRANSITIVELY via another selected pack's
// hard_dep (e.g. release via harness-dogfood -> core/release) is selected,
// never merely "available".
//
// When the resolver errored (renderOK=false), `overlay list` degrades to the
// legacy direct signals: the `overlays:` list, capability-manifest
// dual-selection (e.g. release via `capabilities: [core/release]`), and
// feature-activated shipped pilots. The fallback can over-report a pack that
// fails to render only when the resolver itself errored — but a broken
// catalog/manifest is itself a render blocker the operator must fix regardless
// (render aborts with the same error), so the degraded answer is still
// directionally honest. Capability-manifest reads in the fallback are
// best-effort: a pack without a manifest (skills-only pilots) contributes no
// capability id and falls through to the overlays/features paths.
func (st packSelectionState) isSelected(target, packName string) bool {
	if st.renderOK {
		return st.renderSet[packName]
	}
	// Fallback: resolver failed; degrade to the direct signals.
	for _, o := range st.overlays {
		if o == packName {
			return true
		}
	}
	// Capability-manifest dual-selection (release via core/release, and any
	// future capability-bearing pack). Open the pack the same way a render
	// does (project-local first, then embedded) and read its manifest id.
	if pack, err := overlay.OpenPackFor(target, packName); err == nil {
		if m, ok, _ := pack.ReadCapabilityManifest(); ok && m.ID != "" {
			for _, c := range st.capabilities {
				if string(c) == m.ID {
					return true
				}
			}
		}
	}
	// Shipped default-on pilot (feature-activated).
	for fk, pn := range featurePackMap {
		if pn == packName && st.features[fk] {
			return true
		}
	}
	return false
}

// resolvePackSelection attaches a Selected flag to each enumerated pack by
// testing membership in the canonical render pack-set (loadPackSelection). See
// packSelectionState.isSelected for the selection definition. It never fails: a
// broken profile yields no overlays/capabilities/features and a renderSet read
// error degrades to those legacy signals, so every pack surfaces as available
// in the worst case (the safe direction for discovery).
func resolvePackSelection(target string, packs []overlay.PackInfo) []enrichedPack {
	st := loadPackSelection(target)
	out := make([]enrichedPack, 0, len(packs))
	for _, p := range packs {
		out = append(out, enrichedPack{
			PackInfo: p,
			Selected: st.isSelected(target, p.Name),
		})
	}
	return out
}

// unselectedEmbeddedPacks returns the sorted names of shipped (embedded) packs
// at target that are NOT currently selected (would NOT render on the next
// update). It is the discovery signal guide's ungated hint surfaces so a
// coordinator or operator learns these packs EXIST without a false-negative.
// Returns nil when no embedded pack is unselected, or when the pack list cannot
// be enumerated (the hint simply does not render). Shares loadPackSelection
// with resolvePackSelection so both surfaces project the SAME render pack-set.
func unselectedEmbeddedPacks(target string) []string {
	all, err := overlay.KnownPacksFor(target)
	if err != nil {
		return nil
	}
	st := loadPackSelection(target)
	var out []string
	for _, p := range all {
		if p.Source != overlay.PackSourceEmbedded {
			continue // hint targets shipped (embedded) packs
		}
		if !st.isSelected(target, p.Name) {
			out = append(out, p.Name)
		}
	}
	sort.Strings(out)
	return out
}

// resolveOverlayTarget resolves the project root from a --target flag the same
// way guide.go / overlay docs do: runshape.FindForRoot walks up from the
// resolved-absolute target for an install root; if that fails, fall back to the
// target itself when it is an existing dir; otherwise "" forces embedded-only
// resolution (overlay.OpenPackFor / KnownPacksFor document target="" as
// embedded-only). Shared by `overlay list` and `overlay docs`.
func resolveOverlayTarget(targetFlag string) string {
	target := strings.TrimSpace(targetFlag)
	abs, absErr := filepath.Abs(target)
	if absErr != nil || abs == "" {
		return ""
	}
	if root, _, err := runshape.FindForRoot(abs); err == nil && root != "" {
		return root
	}
	if isExistingDir(abs) {
		return abs
	}
	return ""
}
