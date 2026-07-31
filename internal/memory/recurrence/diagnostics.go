package recurrence

// diagnostics.go — the PURE recurrence-signature DIAGNOSTICS (P1-MEMORY-001
// Slice 4). Slice 2's Derive collapses a card population into canonical groups;
// this file surfaces the four diagnostic categories the memo (efa53fb,
// §Placement "Doctor") requires of the doctor layer:
//
//  1. canonical recurrence groups   — informational (the caller reports these
//     directly from Derive(cards).Groups; nothing here).
//  2. malformed identity            — MalformedBlock: per-card raw-shape defects.
//  3. conflicting aliases           — Diagnose: ambiguous alias map.
//  4. uncollapsed duplicates        — Diagnose: producer-bypass signal.
//
// Authority line (memo §Authority-line engagement): this package INFORMS only.
// Both functions are PURE — no I/O, no store, no side effects. The release gate
// (Slice 5) ACTs / fails closed; doctor consumes these diagnostics advisedly.
//
// Why MalformedBlock takes raw bytes (not a typed Card): the typed Block decodes
// a structurally-absent ack-pair field to its zero value (0), so "the card is
// missing last_acknowledged_count" is indistinguishable from "the count is 0"
// after decode. The ack-pair is a load-bearing contract (the schema REQUIRES
// both whenever a block is present), so the diagnostic must see the RAW shape.
// The full draft-07 schema lives in the Go validator (internal/taskcard, the
// `vh-agent-harness task-card validate` subcommand — the defer-018 port that
// retired the standalone Python script); MalformedBlock replicates only the
// load-bearing observable checks doctor needs at runtime.

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// DiagnosticCategory labels one recurrence-diagnostic DEFECT category (memo
// efa53fb, categories 2-4; category 1 is informational and surfaced by the
// caller directly from Derive, not as a Finding).
type DiagnosticCategory string

const (
	// DiagMalformedIdentity — a card's recurrence block fails a load-bearing
	// shape/consistency check (empty id / bad symptom_class_id pattern /
	// negative count / count<ack / missing ack pair).
	DiagMalformedIdentity DiagnosticCategory = "malformed_identity"
	// DiagConflictingAliases — the alias map is ambiguous: two cards claim
	// canonical over the same alias id, or a cycle Derive had to break.
	DiagConflictingAliases DiagnosticCategory = "conflicting_aliases"
	// DiagUncollapsedDupes — N recurrence-bearing cards sharing an
	// effective_recurrence_id exist as SEPARATE cards (producer dedup bypass).
	DiagUncollapsedDupes DiagnosticCategory = "uncollapsed_duplicates"
)

// Finding is ONE recurrence-diagnostic DEFECT. INFORMS only (memo authority
// line): it is advisory input a caller (doctor) formats; it never authorizes a
// transition. Identity names the recurrence_id / effective id concerned;
// TaskID names a specific card when applicable; Detail is human-readable.
type Finding struct {
	Category DiagnosticCategory
	Identity string
	TaskID   string
	Detail   string
}

// symptomClassIDPattern is the regex anchored on the task-card.schema.json
// recurrence.symptom_class_id pattern ("^recurrence\\.v1/.+$", line 316),
// replicated in Go so the malformed check does not shell out to the full
// validator on every doctor run. It is the load-bearing shape; the full
// draft-07 schema is enforced by internal/taskcard (the defer-018 Go port).
var symptomClassIDPattern = regexp.MustCompile(`^recurrence\.v1/.+$`)

// MalformedBlock returns the load-bearing shape defects in a raw recurrence
// block (the JSON bytes of the card's "recurrence" object). It replicates the
// recurrence-block checks the full contract validator (internal/taskcard) also
// enforces, limited to the load-bearing fields doctor needs at runtime:
//
//   - empty recurrence_id (present-but-empty OR structurally absent),
//   - symptom_class_id not matching ^recurrence.v1/.+$ (present-but-bad OR absent),
//   - negative recurrence_count,
//   - recurrence_count < last_acknowledged_count (the cross-field ack-pair
//     invariant draft-07 cannot express),
//   - a missing / structurally-incomplete acknowledgement pair (the contract
//     requires BOTH counts whenever a block is present).
//
// It does NOT re-implement the full draft-07 schema (type/array/evidence shape
// beyond these load-bearing fields). PURE: no I/O. Returns nil when rawBlock is
// empty/legacy or the block is well-formed. The returned Findings carry only
// Category + Detail; the caller (doctor) attaches Identity/TaskID from the
// decoded card it already holds.
func MalformedBlock(rawBlock []byte) []Finding {
	if len(rawBlock) == 0 {
		return nil // no block → legacy card → not malformed
	}
	// Pointer-typed fields so structurally-absent keys are distinguishable from
	// present-but-zero (the load-bearing reason this takes raw bytes).
	var raw struct {
		RecurrenceID          *string `json:"recurrence_id"`
		SymptomClassID        *string `json:"symptom_class_id"`
		RecurrenceCount       *int    `json:"recurrence_count"`
		LastAcknowledgedCount *int    `json:"last_acknowledged_count"`
	}
	if err := json.Unmarshal(rawBlock, &raw); err != nil {
		// An unparseable recurrence object is malformed shape (the schema would
		// reject it). Surface one finding; defer-liveness owns truly broken
		// whole-card parse errors, but a recurrence object that is JSON-shaped
		// enough to be captured yet fails to decode is a recurrence defect.
		return []Finding{{Category: DiagMalformedIdentity,
			Detail: "recurrence block is not valid JSON: " + cleanJSONErr(err)}}
	}
	var out []Finding

	// Empty recurrence_id (present-but-empty OR absent).
	if raw.RecurrenceID == nil || *raw.RecurrenceID == "" {
		out = append(out, Finding{Category: DiagMalformedIdentity,
			Detail: "recurrence_id is empty (required when a recurrence block is present)"})
	}

	// Bad symptom_class_id pattern (present-but-bad OR absent).
	classVal := ""
	if raw.SymptomClassID != nil {
		classVal = *raw.SymptomClassID
	}
	if raw.SymptomClassID == nil || !symptomClassIDPattern.MatchString(classVal) {
		out = append(out, Finding{Category: DiagMalformedIdentity,
			Detail: fmt.Sprintf("symptom_class_id %q does not match ^recurrence.v1/.+$", classVal)})
	}

	// Acknowledgement pair: both counts required whenever a block is present.
	countPresent := raw.RecurrenceCount != nil
	ackPresent := raw.LastAcknowledgedCount != nil
	if !countPresent || !ackPresent {
		out = append(out, Finding{Category: DiagMalformedIdentity,
			Detail: "acknowledgement pair incomplete: recurrence_count and last_acknowledged_count are both required when a recurrence block is present"})
	} else {
		cnt := *raw.RecurrenceCount
		ack := *raw.LastAcknowledgedCount
		if cnt < 0 {
			out = append(out, Finding{Category: DiagMalformedIdentity,
				Detail: fmt.Sprintf("recurrence_count %d is negative (must be >= 0)", cnt)})
		}
		// Ack has minimum:0 too; report a negative ack explicitly (defensive —
		// the schema rejects it, but a hand-authored card could carry it).
		if ack < 0 {
			out = append(out, Finding{Category: DiagMalformedIdentity,
				Detail: fmt.Sprintf("last_acknowledged_count %d is negative (must be >= 0)", ack)})
		}
		if cnt >= 0 && ack >= 0 && cnt < ack {
			out = append(out, Finding{Category: DiagMalformedIdentity,
				Detail: fmt.Sprintf("recurrence_count %d < last_acknowledged_count %d (ack-pair invariant violated)", cnt, ack)})
		}
	}
	return out
}

// cleanJSONErr trims a json error to its essential message (drops the leading
// positional noise for a readable diagnostic detail).
func cleanJSONErr(err error) string {
	s := err.Error()
	// json.Unmarshal errors look like "json: cannot unmarshal ... into ...";
	// keep them essentially as-is but collapse internal whitespace runs.
	return strings.TrimSpace(s)
}

// Diagnose returns recurrence-diagnostic DEFECTS that need a CROSS-CARD view:
// conflicting aliases (category 3) and uncollapsed duplicates (category 4). It
// consumes the SAME effective-identity + alias reconciliation as Derive
// (buildAliasMap + resolveAlias), so diagnostics and canonical grouping can
// never disagree on identity. PURE: no I/O, no side effects. INFORMS only.
//
// Category 1 (canonical groups) is informational and surfaced by the caller
// directly from Derive(cards).Groups. Category 2 (malformed identity) is
// per-card raw-shape and surfaced by MalformedBlock (the typed Card cannot
// represent a structurally-missing ack pair). Findings are returned grouped by
// category (conflicting aliases first, then uncollapsed duplicates), with
// deterministic ordering within each category.
func Diagnose(cards []Card) []Finding {
	var out []Finding
	out = append(out, diagnoseAliasConflicts(cards)...)
	out = append(out, diagnoseUncollapsedDuplicates(cards)...)
	return out
}

// diagnoseAliasConflicts surfaces category 3: an ambiguous alias map. Three
// signals, all authoring errors Derive papers over (first-declaration-wins +
// silent cycle guard) that the diagnostic must make visible:
//
//   - SELF-ALIAS: a card declares its own recurrence_id as an alias.
//   - CONFLICTING CANONICAL CLAIMS: two DIFFERENT recurrence_ids each declare
//     the SAME alias id as folding into themselves (the alias id cannot fold
//     into two canonicals at once — the map is ambiguous).
//   - CYCLE: a directed cycle in the alias graph (mutual or chained), which
//     resolveAlias terminates on but cannot "correctly" order.
//
// Alias edges are derived exactly as buildAliasMap derives them: for a card
// carrying a block with a non-empty recurrence_id, every alias.RecurrenceID
// points INTO that canonical id. Cards with an empty recurrence_id are skipped
// here (their malformed identity is surfaced by MalformedBlock).
func diagnoseAliasConflicts(cards []Card) []Finding {
	var out []Finding

	// claims[aliasID] = set of canonical recurrence_ids that declare it.
	claims := map[string]map[string]bool{}
	// edges[canonical] = alias ids it declares (the alias graph for cycle
	// detection: an edge canonical -> aliasID).
	edges := map[string][]string{}

	for _, c := range cards {
		if c.Recurrence == nil {
			continue
		}
		canon := c.Recurrence.RecurrenceID
		if canon == "" {
			continue // malformed identity (empty id) — MalformedBlock owns it
		}
		for _, a := range c.Recurrence.Aliases {
			aid := a.RecurrenceID
			if aid == "" {
				continue
			}
			if aid == canon {
				out = append(out, Finding{Category: DiagConflictingAliases, Identity: canon,
					Detail: fmt.Sprintf("recurrence_id %q declares itself as an alias (self-alias)", canon)})
				continue
			}
			if claims[aid] == nil {
				claims[aid] = map[string]bool{}
			}
			claims[aid][canon] = true
			edges[canon] = append(edges[canon], aid)
		}
	}

	// Conflicting canonical claims: one alias id claimed by >= 2 canonicals.
	// Deterministic order: by alias id, then sorted canonical list.
	aliasIDs := make([]string, 0, len(claims))
	for aid := range claims {
		aliasIDs = append(aliasIDs, aid)
	}
	sort.Strings(aliasIDs)
	for _, aid := range aliasIDs {
		canons := claims[aid]
		if len(canons) < 2 {
			continue
		}
		list := make([]string, 0, len(canons))
		for c := range canons {
			list = append(list, c)
		}
		sort.Strings(list)
		out = append(out, Finding{Category: DiagConflictingAliases, Identity: aid,
			Detail: fmt.Sprintf("alias id %q is claimed as canonical by conflicting recurrence_ids: %s", aid, strings.Join(list, ", "))})
	}

	// Cycle detection over the alias graph. Each distinct cycle (by sorted
	// node-set signature) is reported once.
	for _, cyc := range distinctCycles(edges) {
		out = append(out, Finding{Category: DiagConflictingAliases,
			Detail: fmt.Sprintf("alias cycle detected among recurrence_ids: %s (derivation breaks the cycle; reconcile the aliases explicitly)", strings.Join(cyc, " <->"))})
	}

	return out
}

// distinctCycles finds directed cycles in the alias edge graph and returns each
// distinct cycle once, as a sorted node list. A node set that participates in a
// cycle (a back edge in DFS) is captured; duplicate signatures (the same cycle
// reached from different DFS roots) are collapsed. Pure: a function of edges.
func distinctCycles(edges map[string][]string) [][]string {
	const (
		white = 0 // unvisited
		gray  = 1 // on the current DFS path
		black = 2 // fully explored
	)
	color := map[string]int{}
	var stack []string
	onStack := map[string]int{} // node -> index in stack
	seen := map[string]bool{}   // sorted-signature dedup
	var out [][]string

	var dfs func(u string)
	dfs = func(u string) {
		color[u] = gray
		idx := len(stack)
		stack = append(stack, u)
		onStack[u] = idx
		for _, v := range edges[u] {
			switch color[v] {
			case white:
				dfs(v)
			case gray:
				// Back edge: the cycle is stack[onStack[v] : ] (inclusive).
				cyc := append([]string{}, stack[onStack[v]:]...)
				sig := append([]string{}, cyc...)
				sort.Strings(sig)
				key := strings.Join(sig, "|")
				if !seen[key] {
					seen[key] = true
					sort.Strings(cyc)
					out = append(out, cyc)
				}
			}
		}
		color[u] = black
		stack = stack[:idx]
		delete(onStack, u)
	}

	// Iterate DFS roots in sorted order for deterministic output.
	roots := make([]string, 0, len(edges))
	for n := range edges {
		roots = append(roots, n)
	}
	sort.Strings(roots)
	for _, n := range roots {
		if color[n] == white {
			dfs(n)
		}
	}
	return out
}

// diagnoseUncollapsedDuplicates surfaces category 4: N recurrence-bearing cards
// sharing a resolved effective_recurrence_id that exist as SEPARATE cards — the
// producer-bypass signal (memo §Placement "Doctor"). The producer (Slice 3,
// ResolveRecurrence) merges a repeat into the canonical instead of spawning; so
// if >= 2 recurrence-bearing cards resolve to the same effective id, the dedup
// path was bypassed (manual/direct writes).
//
// This reuses the SAME alias reconciliation as Derive (buildAliasMap +
// resolveAlias), so identity here matches canonical grouping exactly. Only
// recurrence-BEARING cards are counted: the producer dedup path applies solely
// to recurrence cards, and a legacy card's effective id is its own unique
// task_id (counting it would false-flag the legacy+recurrence literal-key
// collision the derivation intentionally allows). Cards with an empty
// recurrence_id are skipped (MalformedBlock owns them).
func diagnoseUncollapsedDuplicates(cards []Card) []Finding {
	am := buildAliasMap(cards)
	byID := map[string][]string{} // effective id -> recurrence-bearing task_ids
	for _, c := range cards {
		if c.Recurrence == nil {
			continue
		}
		if c.Recurrence.RecurrenceID == "" {
			continue // malformed identity — MalformedBlock owns it
		}
		eff := resolveAlias(EffectiveID(c), am)
		byID[eff] = append(byID[eff], c.TaskID)
	}

	ids := make([]string, 0, len(byID))
	for id := range byID {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	var out []Finding
	for _, id := range ids {
		tids := byID[id]
		if len(tids) < 2 {
			continue
		}
		sorted := append([]string{}, tids...)
		sort.Strings(sorted)
		out = append(out, Finding{
			Category: DiagUncollapsedDupes,
			Identity: id,
			TaskID:   sorted[0],
			Detail: fmt.Sprintf("%d cards share effective_recurrence_id %q but exist as separate cards (producer dedup bypass): %s — run `vh-agent-harness recurrence dedup` to collapse them",
				len(sorted), id, strings.Join(sorted, ", ")),
		})
	}
	return out
}
