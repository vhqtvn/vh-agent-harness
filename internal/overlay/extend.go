package overlay

// Prompt-extension: managed agent/skill/command prompts carry explicit anchor
// lines (<!-- HARNESS:EXTEND <slot> -->) in their body; an overlay pack may ship
// a snippet per slot (<unit>.extend.<slot>.<ext>, sibling to the unit file).
// At render/apply the harness injects each selected snippet's content at its
// matching anchor. This file owns the load-bearing pieces of that mechanism:
//
//   - ExtensionSnippet: one pack's contribution for one (target, slot).
//   - parseExtensionSnippet: the <base>.extend.<slot>.<ext> filename contract.
//   - Pack.ExtensionSnippets: load snippets from a pack fs.FS.
//   - InjectSlots: a PURE anchor-injection function (testable, no I/O).
//   - InjectExtensionSnippets: the staging-level render-time merge pass.
//
// Semantics (the unified extension model, Slice 2):
//   - Snippet content is overlay_extension by provenance; the managed body keeps
//     its own ownership class (platform_managed). The injection is re-applied on
//     every apply (the managed body is re-rendered clean), so it is idempotent.
//   - Multiple overlays concatenate in declared (pack-list) order.
//   - Missing snippet for an anchor = no-op (the anchor stays as the empty
//     marker comment).
//   - A selected snippet with NO matching anchor = ORPHAN: warn (and/or a
//     proposal entry), never silently dropped.
//
// See the extension model for the full decision tree.

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/vhqtvn/vh-agent-harness/internal/substrate"
)

// ExtensionSnippet is one overlay prompt-extension snippet: a body of content
// to inject at a named anchor slot inside a unit file's body. Snippets are
// render-time merge material, overlay_extension by provenance; the target
// file's own ownership class is unchanged.
type ExtensionSnippet struct {
	// Pack is the name of the overlay pack that contributed the snippet.
	Pack string
	// TargetRel is the pack-relative unit path the snippet extends, WITHOUT the
	// .extend.<slot> infix (e.g. "agents/build.md"). The LIVE path is
	// opencodePrefix+TargetRel.
	TargetRel string
	// Slot is the anchor slot name the snippet binds to (e.g. "custom-verbs").
	// The matching anchor in the target body is the literal line
	// "<!-- HARNESS:EXTEND <slot> -->".
	Slot string
	// Body is the snippet content to inject at the anchor.
	Body string
}

// extensionInfix is the basename infix marking a pack file as a prompt-extension
// snippet rather than a renderable unit. A snippet path has the shape
// <unitBase>.extend.<slot>.<ext>; it extends <unitBase>.<ext> at slot <slot>.
const extensionInfix = ".extend."

// parseExtensionSnippet parses a pack-relative path of the form
// <dir>/<unitBase>.extend.<slot>.<ext> and returns the target unit path
// (<dir>/<unitBase>.<ext>) and the slot name. ok is false if the path does not
// match the snippet shape (no extension, no .extend. infix, or empty
// unitBase/slot). The slot is the literal text between the first .extend.
// occurrence and the final extension.
func parseExtensionSnippet(rel string) (targetRel, slot string, ok bool) {
	dir, base := path.Split(rel)
	ext := path.Ext(base)
	if ext == "" {
		return "", "", false
	}
	stem := strings.TrimSuffix(base, ext) // <unitBase>.extend.<slot>
	idx := strings.Index(stem, extensionInfix)
	if idx <= 0 { // need a non-empty unitBase before the infix
		return "", "", false
	}
	slot = stem[idx+len(extensionInfix):]
	if slot == "" {
		return "", "", false
	}
	targetRel = path.Join(dir, stem[:idx]+ext)
	return targetRel, slot, true
}

// isExtensionSnippet reports whether rel is a prompt-extension snippet file
// (matches <unitBase>.extend.<slot>.<ext>), and therefore NOT a renderable unit.
func isExtensionSnippet(rel string) bool {
	_, _, ok := parseExtensionSnippet(rel)
	return ok
}

// ExtensionSnippets walks the pack for *.extend.<slot>.<ext> snippet files and
// returns them sorted by (TargetRel, Slot, Pack) for deterministic ordering. A
// snippet at agents/build.extend.custom-verbs.md extends agents/build.md at slot
// custom-verbs. Snippet bodies are read verbatim. Merge-content/catalog files
// (opencode-append.jsonc, callable-graph-snippet.md, permission-pack.jsonc) are
// never snippets and are skipped (parseExtensionSnippet rejects them: no .extend.
// infix, or for a file literally named with .extend. they parse on their own
// merits).
func (p *Pack) ExtensionSnippets() ([]ExtensionSnippet, error) {
	var out []ExtensionSnippet
	err := fs.WalkDir(p.FS, ".", func(rel string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		targetRel, slot, ok := parseExtensionSnippet(rel)
		if !ok {
			return nil
		}
		body, err := fs.ReadFile(p.FS, rel)
		if err != nil {
			return fmt.Errorf("overlay %s: read snippet %q: %w", p.Name, rel, err)
		}
		out = append(out, ExtensionSnippet{
			Pack:      p.Name,
			TargetRel: targetRel,
			Slot:      slot,
			Body:      string(body),
		})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("overlay %s: walk snippets: %w", p.Name, err)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].TargetRel != out[j].TargetRel {
			return out[i].TargetRel < out[j].TargetRel
		}
		if out[i].Slot != out[j].Slot {
			return out[i].Slot < out[j].Slot
		}
		return out[i].Pack < out[j].Pack
	})
	return out, nil
}

// extendAnchorPrefix and extendAnchorSuffix delimit a prompt-extension anchor
// line in a managed body: <!-- HARNESS:EXTEND <slot> -->. The slot name is the
// literal text between the prefix and suffix. The anchor MUST occupy its own
// line (leading/trailing whitespace on that line is tolerated).
const (
	extendAnchorPrefix = "<!-- HARNESS:EXTEND "
	extendAnchorSuffix = " -->"
)

// SlotInjection is the per-(target,slot) concatenation of snippet bodies from
// the active packs, in declared (pack-list) order.
type SlotInjection struct {
	// TargetRel is the pack-relative unit path the injection targets (e.g.
	// "agents/build.md").
	TargetRel string
	// Slot is the anchor slot name.
	Slot string
	// Body is the concatenated snippet bodies (blank-line separated), in the
	// declared order of the contributing packs.
	Body string
	// Packs lists the contributing pack names, in declared order.
	Packs []string
}

// InjectSlots returns managedBody with each SlotInjection's Body inserted
// immediately after its matching <!-- HARNESS:EXTEND <slot> --> anchor line. It
// is a PURE function (no I/O) so the anchor-injection contract is unit-testable
// in isolation.
//
// Rules:
//   - Only the FIRST occurrence of a slot's anchor is the injection point; any
//     later duplicate anchor lines for the same slot are left verbatim.
//   - A slot whose Body is empty/whitespace is treated as "no snippet" and is a
//     no-op (its anchor stays as the empty marker), so it is never reported as
//     an orphan.
//   - A slot with a non-empty Body but NO matching anchor is an orphan: it is
//     returned in orphans and the body is left unchanged for that slot.
//   - Multiple slots anchoring the same line inject in slot-iteration order.
func InjectSlots(managedBody string, slots []SlotInjection) (injected string, orphans []string) {
	lines := strings.Split(managedBody, "\n")
	// First-occurrence anchor line index per slot.
	anchorIdx := map[string]int{}
	for i, line := range lines {
		trim := strings.TrimSpace(line)
		if !strings.HasPrefix(trim, extendAnchorPrefix) || !strings.HasSuffix(trim, extendAnchorSuffix) {
			continue
		}
		slot := trim[len(extendAnchorPrefix) : len(trim)-len(extendAnchorSuffix)]
		if slot == "" {
			continue
		}
		if _, seen := anchorIdx[slot]; !seen {
			anchorIdx[slot] = i
		}
	}
	// afterBlocks maps an anchor line index -> the snippet blocks to insert
	// immediately after it, in slot-iteration order (deterministic).
	afterBlocks := map[int][]string{}
	for _, s := range slots {
		body := strings.TrimRight(s.Body, "\n")
		if strings.TrimSpace(body) == "" {
			continue // no snippet -> no-op, anchor stays empty, NOT an orphan
		}
		idx, ok := anchorIdx[s.Slot]
		if !ok {
			orphans = append(orphans, s.Slot)
			continue
		}
		afterBlocks[idx] = append(afterBlocks[idx], body)
	}
	if len(afterBlocks) == 0 {
		return managedBody, orphans
	}
	out := make([]string, 0, len(lines)+8)
	for i, line := range lines {
		out = append(out, line)
		for _, b := range afterBlocks[i] {
			out = append(out, b)
		}
	}
	return strings.Join(out, "\n"), orphans
}

// InjectReport records the outcome of an extension-snippet injection pass over a
// staged tree.
type InjectReport struct {
	// Injected lists each (target, slot) injection that succeeded (the snippet
	// had a matching anchor in its target file).
	Injected []SlotInjection
	// Orphans lists snippets whose target file had NO matching anchor (the
	// target was absent from staging, OR present but carried no such anchor).
	// These are warned by the seam, never silently dropped.
	Orphans []ExtensionSnippet
}

// InjectExtensionSnippets is the render-time merge pass that injects each active
// pack's extension snippets into their target files in staging. It runs AFTER
// the core corpus and overlay unit files are rendered into staging.
//
// For each target file present in staging, the snippets for each slot are
// concatenated in declared pack order and injected at the matching anchor line.
// Multiple packs contributing the same (target, slot) concatenate in pack-list
// order. A slot with no snippet is a no-op. A snippet whose target file has no
// matching anchor is an ORPHAN (returned in report.Orphans) — never silently
// dropped. The seam warns on orphans.
//
// Each snippet body is run through substrate.SubstituteHarnessTokens — the SAME
// 3-token identity pass the core renderers and RenderUnits apply — before it is
// concatenated/injected, so a snippet that carries {{PROJECT_NAME}} (or
// {{PROJECT_SLUG}} / {{COORDINATOR_DIR}}) lands in the managed target file
// RESOLVED, not as a literal placeholder. The injected content never re-resolves
// tokens already present in the managed body (core already substituted those
// during render). Soft placeholders stay literal by design. A nil/empty answers
// map (or a token-free snippet body) is a no-op fast path.
//
// The target file keeps its ownership class (platform_managed for core files);
// injected snippet content is overlay_extension by provenance. The injection is
// a render-time pass re-applied on every apply, so it is idempotent across
// updates.
func InjectExtensionSnippets(staging string, packs []*Pack, answers map[string]string) (InjectReport, error) {
	var report InjectReport

	// Collect per-target slot injections, preserving pack-list order for
	// concatenation. Group key = TargetRel; within, accumulate per-slot bodies
	// and contributing pack names in first-seen order.
	type slotAcc struct {
		body  string
		packs []string
	}
	targetSlots := map[string]map[string]*slotAcc{}
	targetOrder := map[string]int{}
	orderNext := 0
	for _, p := range packs {
		snips, err := p.ExtensionSnippets()
		if err != nil {
			return report, err
		}
		for _, s := range snips {
			if targetSlots[s.TargetRel] == nil {
				targetSlots[s.TargetRel] = map[string]*slotAcc{}
				targetOrder[s.TargetRel] = orderNext
				orderNext++
			}
			acc := targetSlots[s.TargetRel][s.Slot]
			if acc == nil {
				acc = &slotAcc{}
				targetSlots[s.TargetRel][s.Slot] = acc
			}
			if acc.body != "" {
				acc.body += "\n\n"
			}
			// Resolve the 3 canonical identity tokens on the snippet body BEFORE
			// concatenation, mirroring RenderUnits + the core render write site.
			// No-op fast path when answers is empty or the body carries no sentinel.
			acc.body += string(substrate.SubstituteHarnessTokens([]byte(s.Body), answers))
			acc.packs = append(acc.packs, s.Pack)
		}
	}

	// Process targets in first-seen order for deterministic output.
	orderedTargets := make([]string, 0, len(targetSlots))
	for t := range targetSlots {
		orderedTargets = append(orderedTargets, t)
	}
	sort.Slice(orderedTargets, func(i, j int) bool {
		return targetOrder[orderedTargets[i]] < targetOrder[orderedTargets[j]]
	})

	for _, targetRel := range orderedTargets {
		liveRel := opencodePrefix + targetRel
		targetPath := filepath.Join(staging, filepath.FromSlash(liveRel))
		raw, rerr := os.ReadFile(targetPath)
		if rerr != nil {
			// Target not rendered: every slot for this target is an orphan.
			for slot, acc := range targetSlots[targetRel] {
				report.Orphans = append(report.Orphans, ExtensionSnippet{
					Pack: strings.Join(acc.packs, ","), TargetRel: targetRel, Slot: slot, Body: acc.body,
				})
			}
			continue
		}
		// Build the ordered slot list (slots sorted for determinism within a target).
		slotNames := make([]string, 0, len(targetSlots[targetRel]))
		for slot := range targetSlots[targetRel] {
			slotNames = append(slotNames, slot)
		}
		sort.Strings(slotNames)
		slots := make([]SlotInjection, 0, len(slotNames))
		for _, slot := range slotNames {
			acc := targetSlots[targetRel][slot]
			slots = append(slots, SlotInjection{TargetRel: targetRel, Slot: slot, Body: acc.body, Packs: acc.packs})
		}
		injected, orphanSlots := InjectSlots(string(raw), slots)
		orphanSet := map[string]bool{}
		for _, slot := range orphanSlots {
			orphanSet[slot] = true
			acc := targetSlots[targetRel][slot]
			report.Orphans = append(report.Orphans, ExtensionSnippet{
				Pack: strings.Join(acc.packs, ","), TargetRel: targetRel, Slot: slot, Body: acc.body,
			})
		}
		for _, s := range slots {
			if orphanSet[s.Slot] {
				continue
			}
			report.Injected = append(report.Injected, s)
		}
		if injected != string(raw) {
			if werr := os.WriteFile(targetPath, []byte(injected), 0o644); werr != nil {
				return report, fmt.Errorf("overlay: inject snippet into %q: %w", liveRel, werr)
			}
		}
	}
	return report, nil
}
