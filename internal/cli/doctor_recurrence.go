package cli

// doctor_recurrence.go — the recurrence DOCTOR diagnostic (P1-MEMORY-001
// Slice 4). This wires the PURE diagnostics in internal/memory/recurrence
// (Derive for canonical groups, MalformedBlock for per-card shape, Diagnose for
// cross-card defects) into doctor's check surface, reading the card population
// from .local/coordinator/tasks/.
//
// Authority line (memo efa53fb, §Placement + §Authority-line engagement):
// doctor INFORMS only. The check is WARN (defects) / INFO (clean) / SKIP (no
// recurrence-bearing cards), NEVER FAIL — release-gate enforcement is Slice 5.
// A FAIL here would make doctor UNHEALTHY over state Slice 5 owns, contradicting
// the authority split (doctor detects + informs; the release gate ACTs).
//
// Read layer: doctor's existing defer-liveness check reads cards via
// claims.Derive → claims.DeferCard, a defer-liveness typed view that does NOT
// carry the recurrence block. So this file adds a MINIMAL thin read layer that
// decodes each .json under .local/coordinator/tasks/ into recurrence.Card (via
// the json tags on recurrence.Card/Block) AND retains the raw "recurrence"
// sub-object bytes (MalformedBlock needs the raw shape: the typed Block decodes
// a structurally-absent ack-pair field to 0, hiding the "missing ack pair"
// defect). All logic stays in the pure package; this file only reads + formats.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/vhqtvn/vh-agent-harness/internal/memory/recurrence"
)

// recurrenceCardFile is one loaded coordinator task card: its decoded
// recurrence.Card plus the raw JSON bytes of its "recurrence" sub-object (nil
// when the card is legacy / carries no block). BlockRaw lets MalformedBlock
// detect structurally-missing ack fields the typed Block cannot represent.
type recurrenceCardFile struct {
	Path     string
	Card     recurrence.Card
	BlockRaw []byte // raw bytes of the card's "recurrence" object; nil if absent
}

// loadRecurrenceCards is the thin read layer for the recurrence diagnostic. It
// reads every .json file under <target>/.local/coordinator/tasks/ (NO defer-/
// errata- prefix filter: the recurrence block is OPTIONAL on every task-card),
// decodes each into recurrence.Card, and retains the raw recurrence sub-object
// for MalformedBlock. present=false (no error) when the tasks dir is absent — a
// clean SKIP, not a failure.
//
// Individual unreadable or unparseable files are SKIPPED (not errors): this
// check is advisory, and defer-liveness (#12) already owns fail-closed handling
// of unparseable defer/errata cards. A recurrence diagnostic must not fail the
// whole tree over one stray non-card .json.
func loadRecurrenceCards(target string) (files []recurrenceCardFile, present bool, err error) {
	tasksDir := filepath.Join(target, ".local", "coordinator", "tasks")
	entries, readErr := os.ReadDir(tasksDir)
	if readErr != nil {
		if os.IsNotExist(readErr) {
			return nil, false, nil
		}
		return nil, true, readErr
	}
	for _, ent := range entries {
		if ent.IsDir() || !strings.HasSuffix(ent.Name(), ".json") {
			continue
		}
		path := filepath.Join(tasksDir, ent.Name())
		raw, e := os.ReadFile(path)
		if e != nil {
			continue // unreadable file: skip (defer-liveness owns fail-closed card errors)
		}
		// Capture the recurrence sub-object as RawMessage so MalformedBlock can
		// detect structurally-missing ack fields. A card with no "recurrence"
		// key yields a nil RawMessage → legacy card (BlockRaw nil).
		var doc struct {
			TaskID     string          `json:"task_id"`
			Recurrence json.RawMessage `json:"recurrence"`
		}
		if e := json.Unmarshal(raw, &doc); e != nil {
			continue // unparseable card: skip (advisory)
		}
		if doc.TaskID == "" {
			continue
		}
		var card recurrence.Card
		// Best-effort full decode into the typed Card. The decoded Recurrence
		// POINTER gates the malformed loop below (a nil pointer = legacy card or
		// a present-but-non-object recurrence value); for recurrence-bearing
		// cards MalformedBlock inspects BlockRaw for raw shape. Full JSON-shape
		// validation (incl. a non-object recurrence value, which decodes the
		// pointer to nil and is therefore not inspected here) is the
		// authoritative Python validator's job (defer-018, Slice 5); doctor's
		// Go-side check is advisory and covers the load-bearing observable
		// defects. A decode error here is non-fatal.
		_ = json.Unmarshal(raw, &card)
		files = append(files, recurrenceCardFile{
			Path:     path,
			Card:     card,
			BlockRaw: append([]byte(nil), doc.Recurrence...), // copy (doc goes out of scope)
		})
	}
	return files, true, nil
}

// checkRecurrenceState is the 21st doctor check (P1-MEMORY-001 Slice 4). It
// surfaces the four recurrence diagnostic categories (memo efa53fb, §Placement
// "Doctor") over the .local/coordinator/tasks/ card population:
//
//  1. canonical recurrence groups — informational report (identity / symptom
//     class / observation count) from recurrence.Derive.
//  2. malformed identity — a card's recurrence block fails a load-bearing
//     shape/consistency check (recurrence.MalformedBlock over the raw block).
//  3. conflicting aliases — ambiguous alias map (recurrence.Diagnose).
//  4. uncollapsed duplicates — N recurrence-bearing cards sharing an
//     effective_recurrence_id exist as separate cards (producer dedup bypass).
//
// TIERING (advisory-only, NEVER FAIL — doctor INFORMS; Slice 5 enforces):
//   - SKIP when the tasks dir is absent OR no card carries a recurrence block
//     (legacy-only / core-only — nothing to diagnose).
//   - INFO when recurrence cards exist and NO defect is found (the clean path;
//     reports canonical groups).
//   - WARN when any defect (malformed/conflict/uncollapsed) is found, listing
//     each finding by category + the canonical-groups summary.
//
// READ-ONLY: it never mutates a card, never shells out, and never blocks a
// commit or release. The uncollapsed-duplicates check DETECTS producer-bypass in
// the card population; it does NOT itself enforce merging (the producer, Slice
// 3, owns writes; the release gate, Slice 5, owns enforcement).
func checkRecurrenceState(target string) checkResult {
	const name = "recurrence-state"

	files, present, err := loadRecurrenceCards(target)
	if err != nil {
		// Directory-level I/O failure (not IsNotExist) on a source the check
		// needs — surface as a non-blocking WARN (not FAIL): doctor INFORMS, and
		// a recurrence diagnostic must not fail the tree over a read error.
		return checkResult{name: name, tier: tierWarn,
			detail: "could not read recurrence cards under .local/coordinator/tasks/: " + err.Error()}
	}
	if !present {
		return checkResult{name: name, tier: tierSkip,
			detail: "no .local/coordinator/tasks/ dir (nothing to diagnose)"}
	}

	cards := make([]recurrence.Card, 0, len(files))
	for _, f := range files {
		cards = append(cards, f.Card)
	}

	// If no card carries a recurrence block, there is nothing to diagnose:
	// legacy cards have unique task_ids and the recurrence machinery never
	// applies to them. SKIP mirrors doctor's SKIP-when-nothing-to-check
	// convention (skills, auto-classifier when unselected).
	hasBearing := false
	for _, c := range cards {
		if c.Recurrence != nil {
			hasBearing = true
			break
		}
	}
	if !hasBearing {
		return checkResult{name: name, tier: tierSkip,
			detail: fmt.Sprintf("%d card(s) under .local/coordinator/tasks/; none carry a recurrence block (nothing to diagnose)", len(cards))}
	}

	// Category 2: malformed identity (per-card raw shape). Attach the decoded
	// card's task_id + recurrence_id to each finding so the operator can locate
	// the offending card.
	var findings []recurrence.Finding
	for _, f := range files {
		if f.Card.Recurrence == nil || len(f.BlockRaw) == 0 {
			continue
		}
		for _, mf := range recurrence.MalformedBlock(f.BlockRaw) {
			mf.TaskID = f.Card.TaskID
			mf.Identity = f.Card.Recurrence.RecurrenceID
			findings = append(findings, mf)
		}
	}
	// Categories 3 + 4: cross-card defects (conflicting aliases, uncollapsed
	// duplicates). Diagnose reuses the SAME alias reconciliation as Derive.
	findings = append(findings, recurrence.Diagnose(cards)...)

	// Category 1: canonical recurrence groups (informational), from Derive.
	groupsSummary := formatRecurrenceGroups(recurrence.Derive(cards))

	if len(findings) == 0 {
		return checkResult{name: name, tier: tierInfo,
			detail: groupsSummary + " — no defects (advisory only; release enforcement is a later slice)"}
	}
	return checkResult{name: name, tier: tierWarn,
		detail: formatRecurrenceFindings(findings) + "\n  canonical: " + groupsSummary}
}

// formatRecurrenceGroups renders the informational canonical-groups summary
// (category 1) from a Derive Result: one entry per non-legacy canonical group
// (identity / symptom class / observation count), plus a count of legacy cards
// that carry no recurrence block.
func formatRecurrenceGroups(res recurrence.Result) string {
	var b strings.Builder
	recGroups, legacyCards := 0, 0
	var entries []string
	for _, g := range res.Groups {
		if g.IsLegacy {
			legacyCards += len(g.Observations)
			continue
		}
		recGroups++
		entries = append(entries, fmt.Sprintf("%s [class=%s obs=%d]",
			g.EffectiveID, g.SymptomClassID, len(g.Observations)))
	}
	fmt.Fprintf(&b, "%d canonical recurrence group(s)", recGroups)
	if len(entries) > 0 {
		b.WriteString(": ")
		b.WriteString(strings.Join(entries, "; "))
	}
	if legacyCards > 0 {
		fmt.Fprintf(&b, "; %d legacy card(s) without recurrence blocks", legacyCards)
	}
	return b.String()
}

// formatRecurrenceFindings renders the defect findings as a deterministic
// bulleted list (advisory framing). Findings are ordered by category precedence
// (malformed → conflicts → uncollapsed), then identity, then task_id.
func formatRecurrenceFindings(fs []recurrence.Finding) string {
	sort.SliceStable(fs, func(i, j int) bool {
		ci, cj := recurrenceCategoryOrder(fs[i].Category), recurrenceCategoryOrder(fs[j].Category)
		if ci != cj {
			return ci < cj
		}
		if fs[i].Identity != fs[j].Identity {
			return fs[i].Identity < fs[j].Identity
		}
		return fs[i].TaskID < fs[j].TaskID
	})
	var b strings.Builder
	fmt.Fprintf(&b, "%d finding(s) (advisory only; release enforcement is a later slice):", len(fs))
	for _, f := range fs {
		who := ""
		switch {
		case f.TaskID != "":
			who = " " + f.TaskID
		case f.Identity != "":
			who = " " + f.Identity
		}
		fmt.Fprintf(&b, "\n  - %s%s: %s", f.Category, who, f.Detail)
	}
	return b.String()
}

// recurrenceCategoryOrder fixes the display precedence of defect categories
// (most actionable first): malformed identity, then conflicting aliases, then
// uncollapsed duplicates.
func recurrenceCategoryOrder(c recurrence.DiagnosticCategory) int {
	switch c {
	case recurrence.DiagMalformedIdentity:
		return 0
	case recurrence.DiagConflictingAliases:
		return 1
	case recurrence.DiagUncollapsedDupes:
		return 2
	}
	return 99
}
