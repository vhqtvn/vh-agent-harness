package cli

// f1_r1.go — the R1 cross-lane join PRODUCER (Slice 2).
//
// R1 is a deterministic join over EXPLICIT property IDs, lane/producer-act IDs,
// source ancestry, agreements/contradictions/unresolved-gaps, hazard<->symptom
// links, and MERGE/UNION disposition. It is NOT a claims registry or query
// service (HYBRID): the producer takes lane inputs and produces ONE immutable
// joined summary; it does not retain queryable state.
//
// JOIN RULES (deterministic — same inputs always produce the same bytes, hence
// the same semantic digest):
//   - Findings are grouped by PropertyID ACROSS lanes.
//   - A property addressed by >=2 lanes becomes ONE MERGE conclusion (the lanes
//     are joined; sources merged with shared-ancestry collapse).
//   - A property addressed by exactly one lane becomes a UNION conclusion
//     (distinct property; never merged with another lane's conclusion).
//   - Two lanes on the same property with differing Positions record a
//     contradiction (pairwise, sorted by LaneID for determinism).
//   - Sources sharing a Locator OR an AncestryRoot are NOT independent: the
//     producer collapses them into one (recording the shared root), so a
//     shared-ancestry source is never double-counted as independent evidence.
//
// IMMUTABILITY: JoinR1CrossLane is pure — it returns a NEW *F1R1JoinSummary and
// never mutates its inputs. WithNewR1Cycle builds a fresh envelope from a prior
// one + a new join under a NEW synthesis_cycle_id; the prior envelope is left
// untouched (a changed conclusion creates a new cycle, never a mutation).

import (
	"fmt"
	"sort"
	"strings"
)

// R1LaneFinding is one lane's finding about a single property.
type R1LaneFinding struct {
	PropertyID string
	Position   string       // the lane's stance on this property ("" = no stance recorded)
	Sources    []F1R1Source // ancestry-bearing; shared ancestry is collapsed by the join
}

// R1LaneInput is one lane's (producer-act's) contribution to the join.
type R1LaneInput struct {
	LaneID   string
	ActID    string
	Findings []R1LaneFinding
}

// R1HazardInput is a hazard link to attach to a conclusion (optional). The
// producer carries it onto the matching property's conclusion; the validator
// checks the survival chain resolves.
type R1HazardInput struct {
	PropertyID           string
	HazardRef            string
	SymptomRefs          []string
	SourceLocators       []string
	AncestryRoots        []string
	ContradictionRef     string
	GapRef               string
	ConsumingR3OptionIDs []string
	ConsumingPAProbeIDs  []string
}

// R1GapInput is an unresolved aspect to attach to a conclusion (optional).
type R1GapInput struct {
	PropertyID string
	Aspect     string
	Detail     string
}

// JoinR1CrossLane is the pure deterministic R1 join producer. It groups lane
// findings by PropertyID, merges same-property lanes (MERGE), leaves distinct
// properties independent (UNION), collapses shared-ancestry sources, records
// contradictions, and attaches hazards/gaps. It returns a NEW summary and
// never mutates inputs. The returned error list holds producer-rejected inputs
// (empty lane_id / property_id); summary-level structural issues (empty source
// locators, broken hazard chains) are left to ValidateF1Envelope so both the
// producer path and the doctor parse path enforce them identically.
func JoinR1CrossLane(lanes []R1LaneInput, hazards []R1HazardInput, gaps []R1GapInput) (*F1R1JoinSummary, []string) {
	var producerErrs []string

	// Index lanes by LaneID for deterministic contradiction pairing.
	type laneKey struct{ laneID, actID string }
	// Group findings by property.
	type contrib struct {
		lane     laneKey
		position string
		sources  []F1R1Source
	}
	byProperty := map[string][]contrib{}
	propertyOrder := []string{} // deterministic insertion-then-sort
	for li, lane := range lanes {
		if strings.TrimSpace(lane.LaneID) == "" {
			producerErrs = append(producerErrs, fmt.Sprintf("lanes[%d]: empty lane_id", li))
			continue
		}
		lk := laneKey{laneID: lane.LaneID, actID: lane.ActID}
		for fi, f := range lane.Findings {
			if strings.TrimSpace(f.PropertyID) == "" {
				producerErrs = append(producerErrs, fmt.Sprintf("lanes[%d].findings[%d]: empty property_id", li, fi))
				continue
			}
			if _, ok := byProperty[f.PropertyID]; !ok {
				propertyOrder = append(propertyOrder, f.PropertyID)
			}
			byProperty[f.PropertyID] = append(byProperty[f.PropertyID], contrib{lane: lk, position: f.Position, sources: f.Sources})
		}
	}
	sort.Strings(propertyOrder)

	var conclusions []F1R1Conclusion
	for _, prop := range propertyOrder {
		contribs := byProperty[prop]
		// Deterministic lane ordering: sort by laneID then actID.
		sort.Slice(contribs, func(i, j int) bool {
			if contribs[i].lane.laneID != contribs[j].lane.laneID {
				return contribs[i].lane.laneID < contribs[j].lane.laneID
			}
			return contribs[i].lane.actID < contribs[j].lane.actID
		})

		c := F1R1Conclusion{
			ConclusionID:    r1ConclusionID(prop),
			PropertyID:      prop,
			JoinDisposition: F1R1JoinUnion,
		}
		for _, ct := range contribs {
			c.Lanes = appendUnique(c.Lanes, F1R1LaneContrib{LaneID: ct.lane.laneID, ActID: ct.lane.actID, Position: ct.position}, func(a, b F1R1LaneContrib) bool {
				return a.LaneID == b.LaneID && a.ActID == b.ActID
			})
			c.Sources = appendSourcesDedup(c.Sources, ct.sources)
		}
		// Disposition derives from DISTINCT lane keys (after dedup), not the
		// raw contribution count: one lane contributing two findings on the
		// same property is still a single-lane UNION, not a MERGE. This keeps
		// producer output consistent with the validator's merge>=2-lanes rule.
		if len(c.Lanes) >= 2 {
			c.JoinDisposition = F1R1JoinMerge
		}
		// Contradictions: each pair (i<j, sorted) with differing positions.
		for i := 0; i < len(contribs); i++ {
			for j := i + 1; j < len(contribs); j++ {
				if contribs[i].position != contribs[j].position && contribs[i].position != "" && contribs[j].position != "" {
					c.Contradictions = append(c.Contradictions, F1R1Contradiction{
						ContradictionID: r1ContradictionID(prop, contribs[i].lane.laneID, contribs[j].lane.laneID),
						LaneA:           contribs[i].lane.laneID,
						LaneB:           contribs[j].lane.laneID,
						Detail:          fmt.Sprintf("%q vs %q on %s", contribs[i].position, contribs[j].position, prop),
					})
				}
			}
		}
		// Sort sources + lanes for byte-stable output.
		c.Sources = sortSources(c.Sources)
		c.Lanes = sortLanes(c.Lanes)
		conclusions = append(conclusions, c)
	}

	summary := &F1R1JoinSummary{Conclusions: conclusions}
	// Attach hazards + gaps onto their property conclusions (deterministic).
	attachR1Hazards(summary, hazards)
	attachR1Gaps(summary, gaps)
	return summary, producerErrs
}

// WithNewR1Cycle builds a FRESH envelope from a prior one by replacing the R1
// entry with newJoin under newCycleID. The prior envelope is NEVER mutated:
// unchanged family entries are deep-copied; the R1 entry is replaced; a new
// cycle_id is assigned; the digest is re-derived. This is the immutability
// helper — a changed conclusion creates a new synthesis_cycle_id and never
// mutates a prior envelope. Returns the new envelope (caller assigns digest if
// needed; the digest is computed here).
func WithNewR1Cycle(prior *F1SynthesisEnvelope, newCycleID string, newJoin *F1R1JoinSummary) *F1SynthesisEnvelope {
	next := &F1SynthesisEnvelope{
		SchemaVersion:    prior.SchemaVersion,
		SynthesisCycleID: newCycleID,
		Applicability:    prior.Applicability,
		Entries:          make([]F1FamilyEntry, 0, len(prior.Entries)),
	}
	replaced := false
	for _, e := range prior.Entries {
		// Deep-copy the entry HEADER so the new envelope owns its own slice
		// arrays (SourceRefs etc.), not the prior's shared backing array.
		ec := F1FamilyEntry{
			Family:     e.Family,
			Triggered:  e.Triggered,
			EntryID:    e.EntryID,
			SourceRefs: copyStrings(e.SourceRefs),
		}
		if e.Family == F1FamilyR1CrossLaneJoin {
			ec.Triggered = F1TriggeredTriggered
			// Deep-copy the caller's join so a later mutation of newJoin does
			// not leak into the returned envelope (true immutable snapshot).
			ec.R1 = deepCopyR1(newJoin)
			replaced = true
		} else {
			// Deep-copy the family summary so the new envelope owns its content
			// and the prior envelope's pointers are not shared into the new one.
			ec.R1 = deepCopyR1(e.R1)
			ec.R3 = deepCopyR3Fork(e.R3)
			ec.PA = deepCopyPA(e.PA)
		}
		next.Entries = append(next.Entries, ec)
	}
	if !replaced {
		// No prior R1 entry existed; append a fresh one (deep-copied so the
		// caller's newJoin is not shared into the returned envelope).
		next.Entries = append(next.Entries, F1FamilyEntry{
			Family:    F1FamilyR1CrossLaneJoin,
			Triggered: F1TriggeredTriggered,
			EntryID:   "entry-r1",
			R1:        deepCopyR1(newJoin),
		})
	}
	if d, err := next.ComputeDigest(); err == nil {
		next.SemanticDigest = d
	}
	return next
}

// --- producer helpers (deterministic) --------------------------------------

// r1ConclusionID derives a stable, property-scoped conclusion ID. The
// conclusion is ABOUT the property, so the ID is property-scoped: two input
// sets addressing the same property yield the same conclusion ID.
func r1ConclusionID(propertyID string) string {
	s := strings.TrimSpace(propertyID)
	s = strings.NewReplacer(" ", "_", "\t", "_").Replace(s)
	return "R1C-" + s
}

// r1ContradictionID derives a stable contradiction ID from the property and the
// two (sorted) lane IDs, so an F1R1HazardLink.ContradictionRef can resolve to
// it. Lane order is normalized so (a,b) and (b,a) yield the same ID.
func r1ContradictionID(propertyID, laneA, laneB string) string {
	a, b := laneA, laneB
	if a > b {
		a, b = b, a
	}
	return "CONTRA-" + sanitizeID(propertyID) + "-" + sanitizeID(a) + "-" + sanitizeID(b)
}

// r1GapID derives a stable gap ID from its aspect, so an
// F1R1HazardLink.GapRef can resolve to it.
func r1GapID(aspect string) string {
	return "GAP-" + sanitizeID(aspect)
}

func sanitizeID(s string) string {
	s = strings.TrimSpace(s)
	s = strings.NewReplacer(" ", "_", "\t", "_", "/", "_").Replace(s)
	return s
}

// appendSourcesDedup merges srcs into dst, collapsing any sources that share a
// Locator OR an AncestryRoot into one independence class (shared-ancestry
// sources are NOT independent and must not be double-counted). The collapse is
// TRANSITIVELY CLOSED (a fixpoint): if A shares a root with B and B shares a
// root with C, all three collapse to one class — matching the validator's
// all-pairs double-count check exactly, so producer output never trips the
// validator. Each output source carries the unioned ancestry roots of its
// class and the lowest locator (sorted) as its representative locator.
func appendSourcesDedup(dst, srcs []F1R1Source) []F1R1Source {
	all := make([]F1R1Source, 0, len(dst)+len(srcs))
	all = append(all, dst...)
	all = append(all, srcs...)
	// No early return on len<=1: the union-find rebuild below allocates fresh
	// AncestryRoots slices via sortedKeysOfSet, so even a single source is
	// deep-copied and never aliases the caller's backing array (purity).
	n := len(all)
	parent := make([]int, n)
	for i := range parent {
		parent[i] = i
	}
	var find func(int) int
	find = func(x int) int {
		for parent[x] != x {
			parent[x] = parent[parent[x]]
			x = parent[x]
		}
		return x
	}
	union := func(a, b int) {
		ra, rb := find(a), find(b)
		if ra != rb {
			parent[ra] = rb
		}
	}
	// Two sources are in the same independence class if they share a non-empty
	// locator or any non-empty ancestry root.
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			if all[i].Locator != "" && all[i].Locator == all[j].Locator {
				union(i, j)
				continue
			}
			for _, ar := range all[i].AncestryRoots {
				if ar != "" && containsString(all[j].AncestryRoots, ar) {
					union(i, j)
					break
				}
			}
		}
	}
	// Collect each class's unioned roots + locators.
	type class struct {
		roots    map[string]struct{}
		locators map[string]struct{}
	}
	classes := map[int]*class{}
	for i := range all {
		root := find(i)
		cl, ok := classes[root]
		if !ok {
			cl = &class{roots: map[string]struct{}{}, locators: map[string]struct{}{}}
			classes[root] = cl
		}
		for _, ar := range all[i].AncestryRoots {
			if ar != "" {
				cl.roots[ar] = struct{}{}
			}
		}
		if all[i].Locator != "" {
			cl.locators[all[i].Locator] = struct{}{}
		}
	}
	out := make([]F1R1Source, 0, len(classes))
	for _, cl := range classes {
		s := F1R1Source{AncestryRoots: sortedKeysOfSet(cl.roots)}
		// Representative locator = the lowest locator in the class (deterministic).
		locs := sortedKeysOfSet(cl.locators)
		if len(locs) > 0 {
			s.Locator = locs[0]
		}
		out = append(out, s)
	}
	return sortSources(out)
}

func sortedKeysOfSet(m map[string]struct{}) []string {
	if len(m) == 0 {
		return nil
	}
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func attachR1Hazards(summary *F1R1JoinSummary, hazards []R1HazardInput) {
	// Sort a COPY (not the caller's slice) by (propertyID, hazardRef) for
	// determinism — the producer must not mutate its inputs.
	hs := make([]R1HazardInput, len(hazards))
	copy(hs, hazards)
	sort.Slice(hs, func(i, j int) bool {
		if hs[i].PropertyID != hs[j].PropertyID {
			return hs[i].PropertyID < hs[j].PropertyID
		}
		return hs[i].HazardRef < hs[j].HazardRef
	})
	for _, h := range hs {
		for i := range summary.Conclusions {
			if summary.Conclusions[i].PropertyID == h.PropertyID {
				summary.Conclusions[i].Hazards = append(summary.Conclusions[i].Hazards, F1R1HazardLink{
					HazardRef:            h.HazardRef,
					SymptomRefs:          sortedCopyStrings(h.SymptomRefs),
					SourceLocators:       sortedCopyStrings(h.SourceLocators),
					AncestryRoots:        sortedCopyStrings(h.AncestryRoots),
					ContradictionRef:     h.ContradictionRef,
					GapRef:               h.GapRef,
					ConsumingR3OptionIDs: sortedCopyStrings(h.ConsumingR3OptionIDs),
					ConsumingPAProbeIDs:  sortedCopyStrings(h.ConsumingPAProbeIDs),
				})
				break
			}
		}
	}
}

func attachR1Gaps(summary *F1R1JoinSummary, gaps []R1GapInput) {
	// Sort a COPY (not the caller's slice) for determinism + non-mutation.
	gs := make([]R1GapInput, len(gaps))
	copy(gs, gaps)
	sort.Slice(gs, func(i, j int) bool {
		if gs[i].PropertyID != gs[j].PropertyID {
			return gs[i].PropertyID < gs[j].PropertyID
		}
		return gs[i].Aspect < gs[j].Aspect
	})
	for _, g := range gs {
		for i := range summary.Conclusions {
			if summary.Conclusions[i].PropertyID == g.PropertyID {
				summary.Conclusions[i].Gaps = append(summary.Conclusions[i].Gaps, F1R1Gap{
					GapID:  r1GapID(g.Aspect),
					Aspect: g.Aspect,
					Detail: g.Detail,
				})
				break
			}
		}
	}
}

// --- deep-copy helpers (immutability: new envelope owns its content) --------

func deepCopyR1(r1 *F1R1JoinSummary) *F1R1JoinSummary {
	if r1 == nil {
		return nil
	}
	out := &F1R1JoinSummary{Conclusions: make([]F1R1Conclusion, len(r1.Conclusions))}
	for i, c := range r1.Conclusions {
		out.Conclusions[i] = F1R1Conclusion{
			ConclusionID:    c.ConclusionID,
			PropertyID:      c.PropertyID,
			JoinDisposition: c.JoinDisposition,
			Lanes:           copyLaneContribs(c.Lanes),
			Sources:         copySources(c.Sources),
			Agreements:      copyStrings(c.Agreements),
			Contradictions:  copyContradictions(c.Contradictions),
			Gaps:            copyGaps(c.Gaps),
			Hazards:         copyHazards(c.Hazards),
		}
	}
	return out
}

func deepCopyPA(pa *F1PAProbeSummary) *F1PAProbeSummary {
	if pa == nil {
		return nil
	}
	out := &F1PAProbeSummary{Probes: make([]F1PAProbe, len(pa.Probes))}
	for i, p := range pa.Probes {
		out.Probes[i] = F1PAProbe{ProbeID: p.ProbeID, TargetRef: p.TargetRef, Result: p.Result, EvidenceRefs: copyStrings(p.EvidenceRefs)}
	}
	return out
}

// --- tiny deterministic slice helpers --------------------------------------

func containsString(s []string, want string) bool {
	for _, v := range s {
		if v == want {
			return true
		}
	}
	return false
}

func sortedCopyStrings(s []string) []string {
	out := make([]string, len(s))
	copy(out, s)
	sort.Strings(out)
	return out
}

func sortSources(s []F1R1Source) []F1R1Source {
	sort.Slice(s, func(i, j int) bool { return s[i].Locator < s[j].Locator })
	return s
}

func sortLanes(l []F1R1LaneContrib) []F1R1LaneContrib {
	sort.Slice(l, func(i, j int) bool {
		if l[i].LaneID != l[j].LaneID {
			return l[i].LaneID < l[j].LaneID
		}
		return l[i].ActID < l[j].ActID
	})
	return l
}

func appendUnique[T any](dst []T, v T, same func(a, b T) bool) []T {
	for _, e := range dst {
		if same(e, v) {
			return dst
		}
	}
	return append(dst, v)
}

func copyLaneContribs(in []F1R1LaneContrib) []F1R1LaneContrib {
	if in == nil {
		return nil
	}
	out := make([]F1R1LaneContrib, len(in))
	copy(out, in)
	return out
}

func copySources(in []F1R1Source) []F1R1Source {
	if in == nil {
		return nil
	}
	out := make([]F1R1Source, len(in))
	for i, s := range in {
		out[i].Locator = s.Locator
		out[i].AncestryRoots = copyStrings(s.AncestryRoots)
	}
	return out
}

func copyContradictions(in []F1R1Contradiction) []F1R1Contradiction {
	if in == nil {
		return nil
	}
	out := make([]F1R1Contradiction, len(in))
	copy(out, in)
	return out
}

func copyGaps(in []F1R1Gap) []F1R1Gap {
	if in == nil {
		return nil
	}
	out := make([]F1R1Gap, len(in))
	copy(out, in)
	return out
}

func copyHazards(in []F1R1HazardLink) []F1R1HazardLink {
	if in == nil {
		return nil
	}
	out := make([]F1R1HazardLink, len(in))
	for i, h := range in {
		out[i].HazardRef = h.HazardRef
		out[i].SymptomRefs = copyStrings(h.SymptomRefs)
		out[i].SourceLocators = copyStrings(h.SourceLocators)
		out[i].AncestryRoots = copyStrings(h.AncestryRoots)
		out[i].ContradictionRef = h.ContradictionRef
		out[i].GapRef = h.GapRef
		out[i].ConsumingR3OptionIDs = copyStrings(h.ConsumingR3OptionIDs)
		out[i].ConsumingPAProbeIDs = copyStrings(h.ConsumingPAProbeIDs)
	}
	return out
}

func copyStrings(in []string) []string {
	if in == nil {
		return nil
	}
	out := make([]string, len(in))
	copy(out, in)
	return out
}
