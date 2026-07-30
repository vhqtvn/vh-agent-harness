package recurrence

// dedup.go — the PRODUCER/DEDUP decision (P1-MEMORY-001 Slice 3, the WRITE-LAYER
// crux). Slice 2's Derive collapses a population of cards into canonical groups;
// ResolveRecurrence decides, for ONE incoming card against the existing population,
// whether the incoming is a REPEAT of a known canonical (→ the producer updates
// the canonical instead of spawning) or a NEW defect (→ the producer writes a
// fresh card). The producer (JS, templates/core/.opencode/scripts/state-lib.js)
// consults this via the `vh-agent-harness recurrence dedup` subcommand at the
// task-writing boundary.
//
// Authority line (memo efa53fb, §Placement + §Authority-line engagement): the
// producer provides synchronous merge CONVENIENCE and APPLIES the decision;
// neither the derivation nor the producer is transition authority. The release
// gate (Slice 5) ACTs / fails closed on an unadjudicated recurrence. This
// package INFORMS only — ResolveRecurrence has NO I/O and NO side effects.
//
// Scoping (memo §Backward compatibility: "Do NOT auto-hash or auto-merge
// existing cards"): ResolveRecurrence considers only recurrence-BEARING cards
// as merge candidates. A legacy existing card (no block) is never a merge
// target, even if its task_id coincides with an incoming recurrence_id — that
// namespace collision is an authoring error to surface via doctor (Slice 4),
// not a silent write-time merge. A legacy INCOMING card (no block) is always a
// NewCard: its effective id is its own unique task_id, which no recurrence
// canonical shares. This keeps the write boundary from silently upgrading a
// legacy card with recurrence semantics.

// EvidenceKindRecurrenceObservation is the evidence discriminator used to
// record WHICH incoming card produced a repeat observation on the canonical
// (memo: "preserve task_id as the card/report identifier even after recurrence
// identity is added"). It fits the Slice-1 schema (evidence.kind is minLength:1,
// not enum-restricted) — no schema change required.
const EvidenceKindRecurrenceObservation = "recurrence_observation"

// Action is the producer's dedup outcome for one incoming card against the
// existing population.
type Action int

const (
	// NewCard: no existing recurrence canonical shares the incoming effective
	// id. The producer writes a fresh canonical card (persisting its recurrence
	// block).
	NewCard Action = iota
	// Merge: an existing recurrence canonical shares the incoming effective id
	// (directly or via alias). The producer updates that canonical with the
	// merged block instead of spawning a new card.
	Merge
)

// String renders Action for diagnostics and logs.
func (a Action) String() string {
	switch a {
	case NewCard:
		return "new_card"
	case Merge:
		return "merge"
	default:
		return "unknown"
	}
}

// Decision is the dedup decision for one incoming card. The producer (Slice 3)
// APPLIES this at the write boundary; it is NOT transition authority (Slice 5
// release gate enforces).
type Decision struct {
	// Action is the dedup outcome (NewCard | Merge).
	Action Action
	// EffectiveID is the resolved canonical collapse key of the incoming card
	// (recurrence_id, alias-reconciled; or task_id for a legacy incoming card).
	EffectiveID string
	// CanonicalTaskID is, on Merge, the existing canonical card's task_id the
	// producer must update instead of writing a new card. Empty on NewCard.
	CanonicalTaskID string
	// Merged is, on Merge, the updated canonical recurrence Block to persist
	// (canonical recurrence_id retained, incoming evidence folded in, a
	// structured recurrence observation appended, recurrence_count bumped,
	// last_acknowledged_count held so the disposition becomes unacknowledged).
	// Nil on NewCard.
	Merged *Block
	// IncomingObservation is the incoming card's retained observation (task_id
	// + evidence), carried verbatim for the producer/history.
	IncomingObservation Observation
}

// ResolveRecurrence decides whether the incoming card is a REPEAT of an
// existing recurrence canonical (Merge) or a new defect (NewCard). It consumes
// the SAME effective-identity + alias reconciliation as Derive (buildAliasMap +
// resolveAlias), so the write-boundary decision and the derivation grouping can
// never disagree on identity.
//
// On Merge it builds the updated canonical Block per memo §Repeat semantics +
// §Manifest-v2 disposition interaction:
//   - The canonical's recurrence_id and symptom_class_id are RETAINED — UNLESS
//     alias reconciliation promotes the incoming's explicit recurrence_id as
//     the effective canonical (e.g., incoming R2 with aliases:[{R1}] against
//     existing R1 → effective R2). In that case the merged block is RE-POINTED:
//     the resolved effective id (R2) becomes the persisted recurrence_id and the
//     prior canonical id (R1) is recorded as an alias so future lookups resolve.
//   - The incoming card's evidence[] folds into the canonical's evidence[].
//   - A structured recurrence observation (EvidenceKindRecurrenceObservation,
//     ref = incoming task_id) is appended so the repeat is attributable.
//   - recurrence_count is incremented (N→N+1).
//   - last_acknowledged_count is HELD, so recurrence_count > last_acknowledged
//     count after the merge → the disposition becomes unacknowledged (a new
//     observation cannot slip through under a stale ack; fail-closed).
//
// The returned Merged block is a DEEP COPY of the canonical block (independent
// slice headers); mutating it does not touch the input population.
func ResolveRecurrence(existing []Card, incoming Card) Decision {
	// Build the alias map over the FULL population (existing + incoming) so the
	// incoming card's own alias declarations are reconciled at the write
	// boundary — matching how Derive would reconcile them over the complete
	// population. Without the incoming in the map, an incoming block whose
	// aliases[] point at an existing card's recurrence_id would NOT be detected
	// as a merge, spawning a second canonical that Derive later collapses
	// (write-time population disagrees with derivation population).
	all := make([]Card, 0, len(existing)+1)
	all = append(all, existing...)
	all = append(all, incoming)
	am := buildAliasMap(all)
	incomingEff := resolveAlias(EffectiveID(incoming), am)

	dec := Decision{
		Action:              NewCard,
		EffectiveID:         incomingEff,
		IncomingObservation: incomingObservation(incoming),
	}

	// Only recurrence-BEARING cards are merge candidates (see package scoping
	// note): a legacy existing card is never silently upgraded at write time.
	for _, c := range existing {
		if c.Recurrence == nil {
			continue
		}
		if resolveAlias(EffectiveID(c), am) == incomingEff {
			dec.Action = Merge
			dec.CanonicalTaskID = c.TaskID
			dec.Merged = mergeCanonicalBlock(c.Recurrence, incoming, incomingEff)
			return dec
		}
	}
	return dec
}

// incomingObservation builds the retained observation for the incoming card:
// its task_id plus any evidence it carried. Legacy incoming cards (no block)
// yield an observation with task_id only.
func incomingObservation(incoming Card) Observation {
	obs := Observation{TaskID: incoming.TaskID}
	if incoming.Recurrence != nil {
		obs.Evidence = append(obs.Evidence, incoming.Recurrence.Evidence...)
	}
	return obs
}

// mergeCanonicalBlock builds the updated canonical Block from the canonical's
// existing block + the incoming repeat. It deep-copies the canonical block so
// the caller can persist the result without aliasing the input population.
//
// effectiveID is the RESOLVED canonical collapse key (alias-reconciled). When
// alias reconciliation promoted the incoming's explicit recurrence_id as the
// effective canonical (effectiveID != canonical.RecurrenceID), the merged block
// is RE-POINTED: effectiveID becomes the persisted recurrence_id and the prior
// canonical id is recorded as an alias. Without this re-point, the prior
// canonical id persists and the incoming's id is unrepresented — a later repeat
// using the incoming's id (without an alias) would resolve to NewCard, spawning
// a second canonical card and violating the N→1 crux.
func mergeCanonicalBlock(canonical *Block, incoming Card, effectiveID string) *Block {
	// Determine the persisted recurrence_id: use the resolved effective id if
	// it differs from the canonical's current id (re-point); otherwise retain
	// the canonical's id.
	mergedID := canonical.RecurrenceID
	if effectiveID != "" && effectiveID != canonical.RecurrenceID {
		mergedID = effectiveID
	}

	merged := &Block{
		RecurrenceID:          mergedID,
		SymptomClassID:        canonical.SymptomClassID,
		RecurrenceCount:       canonical.RecurrenceCount + 1,   // N→N+1
		LastAcknowledgedCount: canonical.LastAcknowledgedCount, // held → unacknowledged
	}
	// Evidence: canonical's existing evidence (copied) ++ incoming's folded
	// evidence ++ one structured recurrence observation recording the incoming
	// task_id (so the repeat is attributable per "preserve task_id").
	merged.Evidence = append(merged.Evidence, canonical.Evidence...)

	// Aliases: canonical's aliases retained, incoming's folded in.
	aliases := append([]Alias{}, canonical.Aliases...)
	if incoming.Recurrence != nil {
		merged.Evidence = append(merged.Evidence, incoming.Recurrence.Evidence...)
		aliases = append(aliases, incoming.Recurrence.Aliases...)
	}

	// Re-point: if the effective id differs from the canonical's current id,
	// record the prior canonical id as an alias so future lookups resolve.
	if effectiveID != "" && effectiveID != canonical.RecurrenceID {
		aliases = append(aliases, Alias{
			RecurrenceID: canonical.RecurrenceID,
			Note:         "prior canonical id, re-pointed during recurrence merge",
		})
	}

	merged.Aliases = aliases
	merged.Evidence = append(merged.Evidence, Evidence{
		Kind: EvidenceKindRecurrenceObservation,
		Ref:  incoming.TaskID,
	})
	return merged
}
