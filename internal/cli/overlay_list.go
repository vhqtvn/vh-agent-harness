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
// unselected ones. Selection status reuses activeOverlays / parseProfileSelection
// / reconciledFeatures (the same signals guide + doctor consume).

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

A pack is "selected" when ANY of:
  - its name is listed under ` + "`overlays:`" + ` in vh-harness-profile.yml, OR
  - its capability-manifest id is listed under ` + "`capabilities:`" + ` (the
    ` + "`release`" + ` pack is dual-selectable via ` + "`core/release`" + `), OR
  - it is a shipped default-on pilot whose feature key resolves true.

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

// resolvePackSelection attaches a Selected flag to each enumerated pack by
// reading the live profile once (overlays + capabilities) plus the reconciled
// feature map. See packSelected for the selection definition. It never fails:
// a profile that cannot be read yields no overlays/capabilities/features, so
// every pack surfaces as available (the safe direction for discovery).
func resolvePackSelection(target string, packs []overlay.PackInfo) []enrichedPack {
	overlays := activeOverlays(target)
	raw, rerr := os.ReadFile(filepath.Join(target, harnessProfileName))
	if rerr != nil {
		raw = nil
	}
	_, capabilities := parseProfileSelection(raw)
	features := reconciledFeatures(target)

	out := make([]enrichedPack, 0, len(packs))
	for _, p := range packs {
		out = append(out, enrichedPack{
			PackInfo: p,
			Selected: packSelected(target, p.Name, overlays, capabilities, features),
		})
	}
	return out
}

// packSelected reports whether packName would render on the next update for the
// project at target. A pack is selected when ANY of:
//   - its name is listed under `overlays:` (activeOverlays), OR
//   - its capability-manifest id is listed under `capabilities:` (e.g. the
//     release pack is dual-selectable via core/release — both paths converge),
//     OR
//   - it is a shipped default-on pilot whose feature key resolves true
//     (reconciledFeatures + featurePackMap; an explicit `overlays:` entry
//     already matched above and survives an opt-out).
//
// This reuses the SAME signals doctor's shipped-pilots check consumes, so the
// status column never reports a rendering pack as merely "available" (the
// inverse false impression this slice exists to kill). Capability-manifest
// reads are best-effort: a pack without a manifest (skills-only INFORMS pilots)
// contributes no capability id and falls through to the overlays/features paths.
func packSelected(target, packName string, overlays []string, capabilities []resolver.CapabilityID, features map[string]bool) bool {
	for _, o := range overlays {
		if o == packName {
			return true
		}
	}
	// Capability-manifest dual-selection (release via core/release, and any
	// future capability-bearing pack). Open the pack the same way a render
	// does (project-local first, then embedded) and read its manifest id.
	if pack, err := overlay.OpenPackFor(target, packName); err == nil {
		if m, ok, _ := pack.ReadCapabilityManifest(); ok && m.ID != "" {
			for _, c := range capabilities {
				if string(c) == m.ID {
					return true
				}
			}
		}
	}
	// Shipped default-on pilot (feature-activated).
	for fk, pn := range featurePackMap {
		if pn == packName && features[fk] {
			return true
		}
	}
	return false
}

// unselectedEmbeddedPacks returns the sorted names of shipped (embedded) packs
// at target that are NOT currently selected (would NOT render on the next
// update). It is the discovery signal guide's ungated hint surfaces so a
// coordinator or operator learns these packs EXIST without a false-negative.
// Returns nil when no embedded pack is unselected, or when the pack list cannot
// be enumerated (the hint simply does not render).
func unselectedEmbeddedPacks(target string) []string {
	all, err := overlay.KnownPacksFor(target)
	if err != nil {
		return nil
	}
	overlays := activeOverlays(target)
	raw, rerr := os.ReadFile(filepath.Join(target, harnessProfileName))
	if rerr != nil {
		raw = nil
	}
	_, capabilities := parseProfileSelection(raw)
	features := reconciledFeatures(target)

	var out []string
	for _, p := range all {
		if p.Source != overlay.PackSourceEmbedded {
			continue // hint targets shipped (embedded) packs
		}
		if !packSelected(target, p.Name, overlays, capabilities, features) {
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
