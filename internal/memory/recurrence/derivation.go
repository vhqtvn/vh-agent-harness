// Package recurrence implements the PURE recurrence-signature derivation: it
// resolves effective identity, reconciles aliases, collapses cards sharing an
// identity into canonical groups, and indexes groups by symptom class for
// query/reporting. It is the non-authoritative derivation layer below the CLI.
//
// Authority line (memo efa53fb, §Placement): this package INFORMS only. It has
// NO I/O, NO store dependency, and NO side effects — Derive is a pure function
// over typed cards. The producer (Slice 3) writes the canonical entry; the
// release gate (Slice 5) ACTs / fails closed. Neither transition lives here.
// The persisted typed-memory store is explicitly NOT release authority (it is
// fail-open: a missing store reads empty); this package therefore derives from
// card state directly and never consults persisted memory.
//
// Two-level identity model (memo §Decision, load-bearing crux):
//   - recurrence_id  — stable, explicit identity of ONE underlying defect.
//     This is the actual collapse key.
//   - symptom_class_id — immutable, versioned taxonomy identifier aggregating
//     the CLASS of symptom. A shared class does NOT merge
//     distinct defects; it only aggregates them for QUERY.
//
// Effective-identity rule (memo §Backward compatibility):
//
//	effective_recurrence_id = recurrence_id   when the block is present
//	                          task_id          for legacy cards (no block)
//
// Do NOT auto-hash or auto-merge existing cards. task_id is preserved as the
// card/report identifier even after recurrence identity is added.
package recurrence

// Block mirrors the OPTIONAL top-level "recurrence" object in the task-card
// schema (Slice 1 contract, commit 7c7c295). json tags mirror the schema field
// names verbatim so a producer can decode a card's raw JSON into this struct.
// A nil *Block means "legacy card, no recurrence block" → effective identity
// falls back to task_id (backward compatibility).
//
// The ack-pair invariant (recurrence_count >= last_acknowledged_count) is
// enforced by the schema's validator script (draft-07 cannot express it); the
// producer guarantees it on input. The derivation does not re-validate it.
type Block struct {
	RecurrenceID          string     `json:"recurrence_id"`
	SymptomClassID        string     `json:"symptom_class_id"`
	RecurrenceCount       int        `json:"recurrence_count"`
	LastAcknowledgedCount int        `json:"last_acknowledged_count"`
	Evidence              []Evidence `json:"evidence,omitempty"`
	Aliases               []Alias    `json:"aliases,omitempty"`
}

// Evidence mirrors one entry in the recurrence block's evidence[] array (Slice
// 1 schema). It is a non-identity observation attached to the entry: a path, a
// claim id, a capability, an outcome, a commit subject/range, or a later
// root-cause finding. kind+ref are the discriminator+locator; the rest are
// optional context carried verbatim.
type Evidence struct {
	Kind          string `json:"kind"`
	Ref           string `json:"ref"`
	Note          string `json:"note,omitempty"`
	Capability    string `json:"capability,omitempty"`
	Outcome       string `json:"outcome,omitempty"`
	CommitSubject string `json:"commit_subject,omitempty"`
	CommitRange   string `json:"commit_range,omitempty"`
}

// Alias mirrors one entry in the recurrence block's aliases[] array (Slice 1
// schema). It is a bounded alias/supersession reconciliation for an identity
// that must be re-pointed after later evidence. RecurrenceID is the alternate
// id; Superseded marks it as superseded by the canonical recurrence_id.
type Alias struct {
	RecurrenceID string `json:"recurrence_id"`
	Superseded   bool   `json:"superseded,omitempty"`
	Note         string `json:"note,omitempty"`
}

// Card is the minimal typed view of a task card for recurrence derivation:
// task_id (the legacy/report identifier, preserved after collapse) + an
// OPTIONAL recurrence block (absent on legacy cards). The derivation never
// touches the filesystem; a producer (Slice 3) maps on-disk/in-memory cards
// into this view. It intentionally omits the §4.1 closure-kernel fields
// (status, files_in_scope, ...) — those belong to internal/memory/claims, not
// to recurrence identity.
type Card struct {
	TaskID     string `json:"task_id"`
	Recurrence *Block `json:"recurrence,omitempty"` // nil → legacy card → effective identity = TaskID
}

// Observation is one card's contribution to a canonical recurrence group. The
// card's task_id is retained (memo §Backward compatibility: "preserve task_id
// as the card/report identifier even after recurrence identity is added") and
// its evidence entries are carried verbatim for aggregation.
type Observation struct {
	TaskID   string     `json:"task_id"`
	Evidence []Evidence `json:"evidence,omitempty"`
}

// Group is ONE canonical recurrence entry: every card sharing a resolved
// effective identity collapsed into a single group. Each contributing card is
// retained as an Observation (not silently merged away).
type Group struct {
	// EffectiveID is the resolved canonical collapse key (recurrence_id, or
	// task_id for legacy cards). This is the identity downstream consumers
	// (producer, gate) treat as the single canonical defect id.
	EffectiveID string
	// SymptomClassID is the class taxonomy id carried on the block; "" for
	// legacy cards. A shared class does NOT merge groups — it only aggregates
	// them for the BySymptomClass query view (the crux distinction).
	SymptomClassID string
	// Observations holds one entry per collapsed card, in input order. The
	// derivation retains them rather than spawning a single merged card.
	Observations []Observation
	// RecurrenceCount is the DERIVED count of observations in the group
	// (== len(Observations)). It reflects the population the derivation
	// collapsed; the producer (Slice 3) maintains the authored per-card count.
	RecurrenceCount int
	// Aliases are the alias declarations aggregated across the group's cards.
	Aliases []Alias
	// IsLegacy is true when NO card in the group carries a recurrence block
	// (the group exists purely by task_id backward-compat).
	IsLegacy bool
}

// Result is the canonical recurrence grouping produced by Derive.
type Result struct {
	Groups []Group
}

// EffectiveID returns the collapse key for a card (memo §Backward
// compatibility): the recurrence block's recurrence_id when the block is
// present and non-empty, otherwise the task_id (legacy backward-compat). This
// is computed BEFORE alias reconciliation — the raw effective identity of one
// card in isolation.
func EffectiveID(c Card) string {
	if c.Recurrence != nil && c.Recurrence.RecurrenceID != "" {
		return c.Recurrence.RecurrenceID
	}
	return c.TaskID
}

// Derive is the PURE recurrence-signature derivation. It resolves effective
// identity, reconciles aliases, collapses cards sharing a resolved identity
// into ONE canonical group, aggregates observations + count, and lets callers
// query groups by symptom class (Result.BySymptomClass — a query VIEW, not a
// merge). No I/O, no store, no side effects. INFORMS only.
//
// Determinism: the grouping is a pure function of the input. Groups appear in
// first-seen resolved-identity order; observations within a group appear in
// input order; aliases aggregate in first-seen order. Running Derive twice on
// the same input yields the same Result.
//
// Alias directional choice (memo §Decision, "bounded reconciliation"): the
// explicitly-declared recurrence_id of a card carrying an aliases[] block is
// CANONICAL; each alias id folds INTO it. A later card whose effective identity
// matches an alias id reconciles into the canonical group. Finer supersession
// ordering (chains, mutual declarations) is bounded here to single-hop
// resolution with cycle-guarded termination; if it proves complex in practice,
// finer ordering is deferred to a later slice rather than over-built now.
func Derive(cards []Card) Result {
	am := buildAliasMap(cards)

	// Accumulate groups keyed by resolved canonical id, preserving first-seen
	// order for deterministic output.
	var order []string
	type acc struct {
		g         Group
		blockSeen bool // any card in this group carried a recurrence block
	}
	groups := map[string]*acc{}

	for _, c := range cards {
		id := resolveAlias(EffectiveID(c), am)
		a, ok := groups[id]
		if !ok {
			a = &acc{}
			a.g.EffectiveID = id
			groups[id] = a
			order = append(order, id)
		}
		// Observation: retain task_id + evidence verbatim.
		obs := Observation{TaskID: c.TaskID}
		if c.Recurrence != nil {
			a.blockSeen = true
			if a.g.SymptomClassID == "" {
				a.g.SymptomClassID = c.Recurrence.SymptomClassID
			}
			// Copy evidence (value-typed struct → safe independent slice).
			obs.Evidence = append(obs.Evidence, c.Recurrence.Evidence...)
			// Aggregate alias declarations.
			a.g.Aliases = append(a.g.Aliases, c.Recurrence.Aliases...)
		}
		a.g.Observations = append(a.g.Observations, obs)
	}

	res := Result{}
	for _, id := range order {
		a := groups[id]
		a.g.RecurrenceCount = len(a.g.Observations)
		a.g.IsLegacy = !a.blockSeen
		res.Groups = append(res.Groups, a.g)
	}
	return res
}

// BySymptomClass indexes groups by symptom_class_id for QUERY/REPORTING. This
// is a query VIEW over the canonical groups — it does NOT merge distinct
// defects: two groups sharing a class appear together in the slice but remain
// separate canonical entries (the crux distinction: a shared class aggregates
// for reporting only). Legacy groups (no class) are indexed under "".
//
// Ordering within each class slice matches the canonical group order, so the
// query is deterministic.
func (r Result) BySymptomClass() map[string][]Group {
	m := map[string][]Group{}
	for _, g := range r.Groups {
		m[g.SymptomClassID] = append(m[g.SymptomClassID], g)
	}
	return m
}

// buildAliasMap returns aliasID → canonicalID. For each card carrying a
// recurrence block, every alias declared on that block points INTO the block's
// own recurrence_id (the explicitly-declared id is canonical). First
// declaration wins so the map is deterministic regardless of input order.
func buildAliasMap(cards []Card) map[string]string {
	m := map[string]string{}
	for _, c := range cards {
		if c.Recurrence == nil {
			continue
		}
		canon := c.Recurrence.RecurrenceID
		if canon == "" {
			continue
		}
		for _, a := range c.Recurrence.Aliases {
			if a.RecurrenceID == "" {
				continue
			}
			if _, exists := m[a.RecurrenceID]; !exists {
				m[a.RecurrenceID] = canon
			}
		}
	}
	return m
}

// resolveAlias follows the alias map from id to its canonical fixed point. The
// traversal is cycle-guarded: if a cycle is encountered (mutual or chained
// supersession), it terminates at the last id reached rather than looping
// forever. Such pathological cycles are out of scope for this slice; the guard
// only guarantees termination and determinism, not a "correct" supersession
// ordering within a cycle.
func resolveAlias(id string, am map[string]string) string {
	seen := map[string]bool{}
	for {
		if seen[id] {
			return id // cycle: stop deterministically
		}
		seen[id] = true
		next, ok := am[id]
		if !ok || next == "" {
			return id
		}
		id = next
	}
}
