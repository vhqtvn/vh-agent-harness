package cli

// release_gate_recurrence_ack.go — the recurrence-ack RELEASE-GATE ENFORCEMENT
// (P1-MEMORY-001 Slice 5 — the ACTING-AUTHORITY slice). This is the layer where
// the gate FAILS CLOSED on an unacknowledged recurrence (memo efa53fb,
// §"Manifest-v2 disposition interaction"):
//
//   - A recurrence whose DERIVED recurrence_count (read from the live card the
//     producer wrote) exceeds the committed manifest entry's
//     last_acknowledged_count is UNACKNOWLEDGED/UNADJUDICATED → a gate-
//     consistency failure → release BLOCKED until the operator re-adjudicates
//     (bumps the manifest ack to the current count).
//   - An UNCOLLAPSED DUPLICATE (≥2 cards sharing an effective_recurrence_id,
//     existing as separate cards — a producer-bypass signal) is ALSO a gate-
//     consistency failure → BLOCKED.
//
// Authority line (memo §Placement + §Authority-line engagement): the gate ACTs
// (fail-closed). Doctor #21 (checkRecurrenceState) INFORMS. The persisted typed-
// memory store is NOT authority (fail-open). The committed manifest
// (.vh-agent-harness/release-defer-dispositions.json) IS the cross-checkout
// authority for the ack; the live card count is the derived state the producer
// wrote. The count is read from .local/coordinator/tasks/ (losable transport);
// the ack is read from the committed manifest. This enforcement therefore runs
// at DOCTOR time (when .local/ is present), gated on releaseImminent so doctor
// stays HEALTHY during ordinary development (the release ceremony already ran
// doctor before tagging).
//
// BACKWARD-COMPAT (sacred — this is release authority): a manifest entry
// WITHOUT ack fields is unaffected — it is absent from the ack map and the ack
// check is dormant for it. v1 releases do not break. The ack-pair enforcement
// applies ONLY to entries that carry recurrence acknowledgement state.
//
// Reuse: this loads cards via Slice 4's loadRecurrenceCards and derives via
// internal/memory/recurrence.Derive + Diagnose (the SAME pure derivation the
// doctor diagnostic uses), so the gate and doctor can never disagree on identity.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/vhqtvn/vh-agent-harness/internal/memory/recurrence"
)

// recurrenceAckKind labels one gate-consistency failure class.
type recurrenceAckKind string

const (
	ackUnacknowledged  recurrenceAckKind = "unacknowledged"
	ackUncollapsedDupe recurrenceAckKind = "uncollapsed_duplicate"
)

// recurrenceAckBlocker is one release-blocking finding from the recurrence-ack
// enforcement. effectiveID is the canonical recurrence identity the finding
// concerns; detail is the human-readable reason.
type recurrenceAckBlocker struct {
	effectiveID string
	kind        recurrenceAckKind
	detail      string
}

// recurrenceAckReport carries the enforcement output. blockers escalate to FAIL
// only when a release is imminent (checkDeferLiveness maps releaseImminent from
// the about-to-release note count, mirroring the F4-C predicate).
type recurrenceAckReport struct {
	blockers []recurrenceAckBlocker
}

// advisoryDetail renders the ack blockers as a DORMANT advisory block (INFORM
// only). It is called only when the blockers are NOT escalating the tier (the
// active blockers are rendered via formatRecurrenceAckBlockers on the FAIL
// path). Returns "" when there are no blockers. The framing is softer than the
// FAIL rendering so a PASS/dormant detail reads as "pending re-adjudication"
// rather than a release blocker.
func (r recurrenceAckReport) advisoryDetail() string {
	if len(r.blockers) == 0 {
		return ""
	}
	return "recurrence-ack (dormant — no release imminent, INFORM only; resolves before tag time): " +
		formatRecurrenceAckBlockers(r.blockers)
}

// evaluateRecurrenceAck computes the recurrence-ack enforcement surface. It:
//   - loads the recurrence cards from .local/coordinator/tasks/ (reusing Slice
//     4's loadRecurrenceCards — the SAME read layer the doctor diagnostic uses);
//   - derives canonical groups (recurrence.Derive) and cross-card defects
//     (recurrence.Diagnose);
//   - BLOCKs on uncollapsed duplicates (≥2 cards same effective id);
//   - compares each group's derived recurrence_count (the max authored count
//     across the group's cards — the producer-bumped counter) against the
//     committed manifest entry's last_acknowledged_count, BLOCKing when
//     count > ack (unacknowledged).
//
// It never returns an error: a missing/unreadable tasks dir or manifest yields
// an empty report (no blockers) — the ack enforcement is dormant, not failed.
// Doctor #21 already owns the advisory WARN over recurrence card defects; this
// gate only ACTs at release time. A recurrence group whose manifest entry
// carries NO ack fields (v1 backward-compat) is skipped — no ack comparison
// applies, and the explicit-disposition path is unaffected.
func evaluateRecurrenceAck(target string) recurrenceAckReport {
	rep := recurrenceAckReport{}

	files, present, err := loadRecurrenceCards(target)
	if err != nil || !present {
		return rep // nothing to enforce (advisory: doctor #21 owns the WARN)
	}
	cards := make([]recurrence.Card, 0, len(files))
	cardByTaskID := map[string]recurrence.Card{}
	for _, f := range files {
		cards = append(cards, f.Card)
		// Last-write-wins on a duplicate task_id is fine here: the count lookup
		// takes a max, and uncollapsed duplicates are reported independently by
		// Diagnose below.
		cardByTaskID[f.Card.TaskID] = f.Card
	}

	// Uncollapsed duplicates (producer-bypass signal): ≥2 recurrence-bearing
	// cards sharing a resolved effective_recurrence_id. Reuses the SAME alias
	// reconciliation as Derive, so identity matches canonical grouping exactly.
	for _, f := range recurrence.Diagnose(cards) {
		if f.Category != recurrence.DiagUncollapsedDupes {
			continue
		}
		rep.blockers = append(rep.blockers, recurrenceAckBlocker{
			effectiveID: f.Identity,
			kind:        ackUncollapsedDupe,
			detail:      f.Detail,
		})
	}

	// Unacknowledged recurrence: derived count (from live cards) > committed
	// manifest ack. The manifest is the cross-checkout authority for the ack;
	// the card count is the derived state the producer wrote.
	manifestAck := manifestRecurrenceAck(target)
	res := recurrence.Derive(cards)
	// Deterministic blocker order: by effective id.
	var orderedGroups []recurrence.Group
	orderedGroups = append(orderedGroups, res.Groups...)
	sort.SliceStable(orderedGroups, func(i, j int) bool {
		return orderedGroups[i].EffectiveID < orderedGroups[j].EffectiveID
	})
	for _, g := range orderedGroups {
		if g.IsLegacy {
			continue // legacy cards (no recurrence block) are not ack-checked
		}
		// Resolve the committed ack for this group. The CANONICAL keying is by
		// the recurrence_id (the group's EffectiveID): a promoted (ack-carrying)
		// manifest entry's defer_id IS its effective recurrence_id (memo
		// §Backward compatibility: "explicit promotion to the v2 recurrence form
		// before it can be acknowledged"). The gate is ALSO robust to a writer
		// keying the entry by a CONTRIBUTING TASK_ID — a natural v1 habit, since
		// legacy manifest entries are 1:1 by task_id: it checks every candidate
		// key and, for fail-closed conservatism, takes the MINIMUM ack across all
		// matches (a stale ack on ANY matching entry blocks). This closes the
		// fail-open gap where a task_id-keyed promoted entry would otherwise miss
		// the EffectiveID lookup and silently pass under a stale ack.
		ack, hasAck := manifestAck[g.EffectiveID]
		for _, obs := range g.Observations {
			if a, ok := manifestAck[obs.TaskID]; ok {
				if !hasAck || a < ack {
					ack = a
				}
				hasAck = true
			}
		}
		if !hasAck {
			// No ack fields on any matching entry (v1 backward-compat) → the ack
			// check is dormant for this group. The explicit-disposition path
			// (F4-C predicate / release-mode evaluator) is unaffected.
			continue
		}
		// Derived count = the max authored recurrence_count across the group's
		// recurrence-bearing cards. For a properly-deduped group (one card) this
		// is that card's producer-bumped counter; Derive's population count
		// (len Observations) is a different quantity and is NOT used here.
		derived := 0
		sawCount := false
		for _, obs := range g.Observations {
			c, ok := cardByTaskID[obs.TaskID]
			if !ok || c.Recurrence == nil {
				continue
			}
			if !sawCount || c.Recurrence.RecurrenceCount > derived {
				derived = c.Recurrence.RecurrenceCount
				sawCount = true
			}
		}
		if sawCount && derived > ack {
			rep.blockers = append(rep.blockers, recurrenceAckBlocker{
				effectiveID: g.EffectiveID,
				kind:        ackUnacknowledged,
				detail: fmt.Sprintf("recurrence_count %d > last_acknowledged_count %d (unacknowledged — re-adjudicate before release: bump the manifest entry's last_acknowledged_count to the current count)",
					derived, ack),
			})
		}
	}
	return rep
}

// manifestRecurrenceAck reads the committed manifest at
// .vh-agent-harness/release-defer-dispositions.json and returns defer_id →
// last_acknowledged_count for the records that CARRY a recurrence
// acknowledgement pair. Pointer-typed decode fields make a structurally-absent
// ack field distinguishable from a present-but-zero one (the load-bearing
// reason): a record WITHOUT last_acknowledged_count is a v1 entry and is
// absent from the map → the ack check is dormant for it (backward-compat).
//
// A missing or unparseable manifest yields an empty map (the ack check is
// dormant, not failed — the explicit-disposition path is unaffected, and the
// release-mode evaluator owns handshake/schema validation).
func manifestRecurrenceAck(repoRoot string) map[string]int {
	out := map[string]int{}
	path := filepath.Join(repoRoot, ".vh-agent-harness", "release-defer-dispositions.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		return out
	}
	var obj struct {
		Records []struct {
			DeferID               string `json:"defer_id"`
			LastAcknowledgedCount *int   `json:"last_acknowledged_count"`
		} `json:"records"`
	}
	if json.Unmarshal(raw, &obj) != nil {
		return out
	}
	for _, r := range obj.Records {
		if r.DeferID == "" || r.LastAcknowledgedCount == nil {
			continue
		}
		out[r.DeferID] = *r.LastAcknowledgedCount
	}
	return out
}

// formatRecurrenceAckBlockers renders the ack blockers as a labeled FAIL
// detail block. Blockers are ordered by kind (unacknowledged first, then
// uncollapsed duplicates) then effective id for deterministic output.
func formatRecurrenceAckBlockers(blockers []recurrenceAckBlocker) string {
	ordered := append([]recurrenceAckBlocker(nil), blockers...)
	sort.SliceStable(ordered, func(i, j int) bool {
		ki, kj := ackKindOrder(ordered[i].kind), ackKindOrder(ordered[j].kind)
		if ki != kj {
			return ki < kj
		}
		return ordered[i].effectiveID < ordered[j].effectiveID
	})
	var unack, dupe []recurrenceAckBlocker
	for _, b := range ordered {
		switch b.kind {
		case ackUnacknowledged:
			unack = append(unack, b)
		case ackUncollapsedDupe:
			dupe = append(dupe, b)
		}
	}
	var b strings.Builder
	if len(unack) > 0 {
		fmt.Fprintf(&b, "%d unacknowledged recurrence(s) — derived count exceeds the committed last_acknowledged_count (release-authority fail-closed; re-adjudicate before release):",
			len(unack))
		for _, blk := range unack {
			fmt.Fprintf(&b, "\n  - %s: %s", blk.effectiveID, blk.detail)
		}
	}
	if len(dupe) > 0 {
		if len(unack) > 0 {
			b.WriteByte('\n')
		}
		fmt.Fprintf(&b, "%d uncollapsed duplicate recurrence(s) — producer dedup bypass (release-authority fail-closed; collapse them before release):",
			len(dupe))
		for _, blk := range dupe {
			fmt.Fprintf(&b, "\n  - %s: %s", blk.effectiveID, blk.detail)
		}
	}
	return b.String()
}

// ackKindOrder fixes the display precedence of ack blocker kinds (most
// actionable first): unacknowledged, then uncollapsed duplicates.
func ackKindOrder(k recurrenceAckKind) int {
	switch k {
	case ackUnacknowledged:
		return 0
	case ackUncollapsedDupe:
		return 1
	}
	return 99
}
