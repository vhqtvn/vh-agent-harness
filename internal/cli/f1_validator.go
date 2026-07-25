package cli

// F1 pure validator (Slice 1). Pure core: no filesystem, no network, no
// transitions, no report writes. Given an in-memory F1SynthesisEnvelope it
// returns the list of structural-consistency errors (empty == structurally
// consistent). Structural consistency is NOT semantic truth: a structurally
// valid envelope may still describe conclusions that are not actually true —
// proving truth is the federated verifier's job, not a structural gate.
//
// The validator is fail-closed: any error forces validation_disposition =
// incomplete, and only a clean envelope is complete. This is the discipline
// that makes "an applicable seam with a missing entry" incomplete rather than
// passing — it does NOT reuse behavioral-closure's absent-token-passes
// behavior.

import (
	"fmt"
	"sort"
	"strings"
)

// ValidateF1Envelope returns the structural-consistency errors for env AS
// CARRIED BY A COMMITTED ARTIFACT (the doctor parse-site path). It runs the
// canonical-content checks AND verifies the carried validation disposition is
// a member of the closed enum — a committed artifact is untrusted input, so
// an arbitrary disposition string must be rejected just like any other enum.
// The returned slice is empty iff env is structurally consistent. Pure: no
// filesystem, no mutation.
//
// Producers building a fresh envelope should call AssignF1Validation instead:
// it validates canonical content WITHOUT rejecting the not-yet-assigned
// (zero-value) disposition, then sets the disposition from the error count.
// Calling ValidateF1Envelope on a producer's zero-value Validation would
// reject the empty disposition — that is the correct defense at the doctor
// parse site (committed artifacts must carry a valid disposition) but the
// WRONG behavior on the producer path (which is about to overwrite it).
func ValidateF1Envelope(env *F1SynthesisEnvelope) []string {
	errs := validateF1EnvelopeContent(env)
	if env != nil {
		if _, ok := f1ValidValidationDispositions[env.Validation.Disposition]; !ok {
			errs = append(errs, fmt.Sprintf("unknown validation disposition %q (want one of %s)", env.Validation.Disposition, f1SortedKeys(f1ValidValidationDispositions)))
		}
	}
	return errs
}

// validateF1EnvelopeContent returns the canonical-content structural errors
// for env (vocabulary, identity, family binding, cross-references, digest) —
// everything EXCEPT the carried validation disposition. It is the shared core
// for ValidateF1Envelope (doctor parse site, which additionally checks the
// carried disposition) and AssignF1Validation (producer path, which is about
// to OVERWRITE the disposition and so must not reject the not-yet-assigned
// zero value). Pure: no filesystem, no mutation.
func validateF1EnvelopeContent(env *F1SynthesisEnvelope) []string {
	if env == nil {
		return []string{"envelope is nil"}
	}
	var errs []string

	// 1. Closed-vocabulary rejection at the envelope level.
	if _, ok := f1ValidApplicabilities[env.Applicability]; !ok {
		errs = append(errs, fmt.Sprintf("unknown applicability %q (want one of %s)", env.Applicability, f1SortedKeys(f1ValidApplicabilities)))
	}
	// Required envelope-level identity fields (covered by the digest; must be
	// non-empty so the digest is anchored on real identity, not "").
	if env.SchemaVersion == "" {
		errs = append(errs, "schema_version is empty")
	}
	if env.SynthesisCycleID == "" {
		errs = append(errs, "synthesis_cycle_id is empty")
	}

	// 2. Per-entry structural checks (run regardless of applicability so a
	//    malformed entry is always reported, even on a not_triggered
	//    envelope that happens to carry entries).
	entryIDs := map[string]int{}
	for i := range env.Entries {
		e := &env.Entries[i]
		pe := fmt.Sprintf("entries[%d]", i)
		if _, ok := f1ValidFamilies[e.Family]; !ok {
			errs = append(errs, fmt.Sprintf("%s: unknown family %q (want one of %s)", pe, e.Family, f1SortedKeys(f1ValidFamilies)))
		}
		if _, ok := f1ValidTriggered[e.Triggered]; !ok {
			errs = append(errs, fmt.Sprintf("%s: unknown triggered %q (want one of %s)", pe, e.Triggered, f1SortedKeys(f1ValidTriggered)))
		}
		if e.EntryID == "" {
			errs = append(errs, fmt.Sprintf("%s: empty entry_id", pe))
		} else if prev, dup := entryIDs[e.EntryID]; dup {
			errs = append(errs, fmt.Sprintf("%s: duplicate entry_id %q (also at entries[%d])", pe, e.EntryID, prev))
		} else {
			entryIDs[e.EntryID] = i
		}
		if dup := firstDuplicate(e.SourceRefs); dup != "" {
			errs = append(errs, fmt.Sprintf("%s: duplicate source_ref %q", pe, dup))
		}
		// Family↔summary binding: each entry carries ONLY its matching family's
		// summary pointer (an r1 entry carries R1, never R3/PA; etc.). A foreign
		// summary is a structural malformation. A triggered entry MUST carry its
		// matching summary; a not_triggered OR not_applicable entry MUST NOT (it
		// produced no synthesis content). See validateF1FamilyBinding.
		errs = append(errs, validateF1FamilyBinding(pe, e)...)
		// Validate only the matching family's summary (foreign summaries are
		// already flagged by the binding check; running their validators would
		// double-report).
		switch e.Family {
		case F1FamilyR1CrossLaneJoin:
			errs = append(errs, validateR1Summary(pe, e.R1)...)
		case F1FamilyR3RedesignFork:
			errs = append(errs, validateR3Summary(pe, e.R3)...)
		case F1FamilyPACounterEvidence:
			errs = append(errs, validatePASummary(pe, e.PA)...)
		}
	}

	// 3. Exactly-one-entry-per-family when applicable.
	if env.Applicability == F1ApplicabilityRequired {
		seen := map[string]int{}
		for i := range env.Entries {
			f := env.Entries[i].Family
			if prev, dup := seen[f]; dup {
				errs = append(errs, fmt.Sprintf("duplicate family %q (entries[%d] and entries[%d])", f, prev, i))
			} else {
				seen[f] = i
			}
		}
		for _, want := range []string{F1FamilyR1CrossLaneJoin, F1FamilyR3RedesignFork, F1FamilyPACounterEvidence} {
			if _, ok := seen[want]; !ok {
				errs = append(errs, fmt.Sprintf("applicability=required but family %q is missing (exactly one entry per family required)", want))
			}
		}
	}

	// 4. Cross-reference resolution across families. R3 SupportRefs must
	//    resolve to R1 conclusion IDs; R3 CounterEvidenceProbeRefs must
	//    resolve to P-a probe IDs; P-a TargetRef must resolve to an R1
	//    conclusion ID or an R3 option ID. Resolution is structural (the ID
	//    exists), not semantic (the link is meaningful).
	errs = append(errs, resolveF1CrossRefs(env)...)

	// 5. P-a target coverage (memo L211-213, L274 — require-P-a-target-
	//    coverage). When the envelope is applicable, every material R1
	//    conclusion + every declared R3 option must have >=1 coverage-
	//    satisfying probe (result != not_run). This is the envelope-level
	//    instance of the gate; the standalone gate lives in f1_pa_gate.go.
	//    not_run does NOT satisfy coverage (memo L206); unavailable is
	//    structurally valid coverage but cannot support proven (memo L309).
	errs = append(errs, validateEnvelopePACoverage(env)...)

	// 6. Semantic-digest re-derivation. If a digest is stored, recompute the
	//    canonical digest and require equality. An empty digest is itself an
	//    error (the producer must populate it before emit).
	if env.SemanticDigest == "" {
		errs = append(errs, "semantic_digest is empty (producer must compute and assign it)")
	} else {
		got, derr := env.ComputeDigest()
		if derr != nil {
			errs = append(errs, fmt.Sprintf("semantic_digest re-derivation failed: %v", derr))
		} else if got != env.SemanticDigest {
			errs = append(errs, fmt.Sprintf("semantic_digest mismatch: stored %q != recomputed %q (canonical content changed without re-derivation)", env.SemanticDigest, got))
		}
	}

	return errs
}

// AssignF1Validation validates canonical CONTENT (not the carried disposition
// — it is about to be overwritten), assigns the fail-closed disposition +
// error list onto env.Validation, and returns the errors. This is the bridge
// producers (and Slice 5 emit) call after populating content + digest. It
// validates content via validateF1EnvelopeContent (NOT ValidateF1Envelope, the
// doctor parse-site path, which would reject the producer's not-yet-assigned
// zero-value disposition). It mutates ONLY env.Validation (assessment), never
// canonical content.
//
// Producer single-call contract: a valid envelope (content + digest, Validation
// at its zero value) reaches disposition=complete via ONE call. There is no
// second-call requirement and no caller pre-seeding of the disposition — that
// would be circular (validation determines disposition, not vice versa).
func AssignF1Validation(env *F1SynthesisEnvelope) []string {
	errs := validateF1EnvelopeContent(env)
	v := F1ValidationInfo{Disposition: F1ValidationComplete}
	if len(errs) > 0 {
		v.Disposition = F1ValidationIncomplete
		v.Errors = errs
	}
	if env != nil {
		env.Validation = v
	}
	return errs
}

// --- family↔summary binding ------------------------------------------------

// validateF1FamilyBinding enforces that an entry carries ONLY the summary
// matching its family AND that the summary's presence matches the entry's
// triggered state. An r1_cross_lane_join entry may carry R1 (never R3/PA);
// r3_redesign_fork may carry R3 (never R1/PA); pa_counter_evidence may carry
// PA (never R1/R3). A foreign summary is a structural malformation — it would
// let a misplaced summary silently satisfy cross-references. A triggered entry
// MUST carry its matching summary (there is content to summarize); a
// not_triggered OR not_applicable entry MUST NOT carry its matching summary
// (the family did not fire or does not apply, so it produced no synthesis
// content — a summary present on a non-triggered entry would be misleading
// state). This is the PROHIBITIVE reading the DTO doc (f1_envelope.go
// F1Triggered) commits to; code and doc agree. Per amended memo L113.
func validateF1FamilyBinding(pe string, e *F1FamilyEntry) []string {
	hasR1 := e.R1 != nil
	hasR3 := e.R3 != nil
	hasPA := e.PA != nil
	var errs []string
	switch e.Family {
	case F1FamilyR1CrossLaneJoin:
		if hasR3 {
			errs = append(errs, pe+": r1 entry must not carry an r3 summary (foreign summary)")
		}
		if hasPA {
			errs = append(errs, pe+": r1 entry must not carry a pa summary (foreign summary)")
		}
		if e.Triggered == F1TriggeredTriggered && !hasR1 {
			errs = append(errs, pe+": r1 entry is triggered but its r1 summary is missing")
		}
		if e.Triggered != F1TriggeredTriggered && hasR1 {
			errs = append(errs, pe+": r1 entry is "+e.Triggered+" but carries an r1 summary (a non-triggered family produced no synthesis content)")
		}
	case F1FamilyR3RedesignFork:
		if hasR1 {
			errs = append(errs, pe+": r3 entry must not carry an r1 summary (foreign summary)")
		}
		if hasPA {
			errs = append(errs, pe+": r3 entry must not carry a pa summary (foreign summary)")
		}
		if e.Triggered == F1TriggeredTriggered && !hasR3 {
			errs = append(errs, pe+": r3 entry is triggered but its r3 summary is missing")
		}
		if e.Triggered != F1TriggeredTriggered && hasR3 {
			errs = append(errs, pe+": r3 entry is "+e.Triggered+" but carries an r3 summary (a non-triggered family produced no synthesis content)")
		}
	case F1FamilyPACounterEvidence:
		if hasR1 {
			errs = append(errs, pe+": pa entry must not carry an r1 summary (foreign summary)")
		}
		if hasR3 {
			errs = append(errs, pe+": pa entry must not carry an r3 summary (foreign summary)")
		}
		if e.Triggered == F1TriggeredTriggered && !hasPA {
			errs = append(errs, pe+": pa entry is triggered but its pa summary is missing")
		}
		if e.Triggered != F1TriggeredTriggered && hasPA {
			errs = append(errs, pe+": pa entry is "+e.Triggered+" but carries a pa summary (a non-triggered family produced no synthesis content)")
		}
	}
	return errs
}

// --- per-family summary validation -----------------------------------------

func validateR1Summary(pe string, r1 *F1R1JoinSummary) []string {
	if r1 == nil {
		return nil
	}
	var errs []string
	cids := map[string]int{}
	pids := map[string]int{} // property_id uniqueness: two conclusions on the same property = an un-merged join
	for i := range r1.Conclusions {
		c := &r1.Conclusions[i]
		cep := fmt.Sprintf("%s.r1.conclusions[%d]", pe, i)
		if c.ConclusionID == "" {
			errs = append(errs, cep+": empty conclusion_id")
		} else if prev, dup := cids[c.ConclusionID]; dup {
			errs = append(errs, fmt.Sprintf("%s: duplicate conclusion_id %q (also at conclusions[%d])", cep, c.ConclusionID, prev))
		} else {
			cids[c.ConclusionID] = i
		}
		if c.PropertyID == "" {
			errs = append(errs, cep+": empty property_id")
		} else if prev, dup := pids[c.PropertyID]; dup {
			errs = append(errs, fmt.Sprintf("%s: duplicate property_id %q (also at conclusions[%d]) — two conclusions on the same property should be one MERGE join", cep, c.PropertyID, prev))
		} else {
			pids[c.PropertyID] = i
		}
		// Join disposition enum.
		if _, ok := f1ValidR1JoinDispositions[c.JoinDisposition]; !ok {
			errs = append(errs, fmt.Sprintf("%s: unknown join_disposition %q (want one of %s)", cep, c.JoinDisposition, f1SortedKeys(f1ValidR1JoinDispositions)))
		}
		// MERGE requires >=2 lanes; UNION must not carry >1 lane.
		if c.JoinDisposition == F1R1JoinMerge && len(c.Lanes) < 2 {
			errs = append(errs, fmt.Sprintf("%s: join_disposition=merge but %d lane(s) (merge requires >=2 lanes on the same property)", cep, len(c.Lanes)))
		}
		if c.JoinDisposition == F1R1JoinUnion && len(c.Lanes) > 1 {
			errs = append(errs, fmt.Sprintf("%s: join_disposition=union but %d lane(s) (use merge for >=2 lanes on the same property)", cep, len(c.Lanes)))
		}
		// Evidence-bearing conclusions require real source locators. A gap-only
		// conclusion (no lanes/agreements/contradictions/hazards) may omit them.
		evidenceBearing := len(c.Lanes) > 0 || len(c.Agreements) > 0 || len(c.Contradictions) > 0 || len(c.Hazards) > 0
		if evidenceBearing {
			if len(c.Sources) == 0 {
				errs = append(errs, cep+": evidence-bearing conclusion has no sources (missing source locators)")
			}
			for j := range c.Sources {
				if c.Sources[j].Locator == "" {
					errs = append(errs, fmt.Sprintf("%s.sources[%d]: empty locator (evidence-bearing conclusion requires real source locators)", cep, j))
				}
			}
		}
		// Shared-ancestry double-count: two sources sharing a locator or an
		// ancestry root are NOT independent and must have been collapsed.
		for j := 0; j < len(c.Sources); j++ {
			for k := j + 1; k < len(c.Sources); k++ {
				if c.Sources[j].Locator != "" && c.Sources[j].Locator == c.Sources[k].Locator {
					errs = append(errs, fmt.Sprintf("%s.sources[%d,%d]: duplicate locator %q (shared source double-counted as independent)", cep, j, k, c.Sources[j].Locator))
				}
				for _, ar := range c.Sources[j].AncestryRoots {
					if ar != "" && containsString(c.Sources[k].AncestryRoots, ar) {
						errs = append(errs, fmt.Sprintf("%s.sources[%d,%d]: shared ancestry root %q (sources not independent; should be collapsed)", cep, j, k, ar))
					}
				}
			}
		}
		// Hazard survival-chain integrity (within the conclusion). Cross-family
		// refs (consuming R3 option / P-a probe IDs) are resolved in
		// resolveF1CrossRefs so they cover both producer and doctor paths.
		declaredLocators := map[string]struct{}{}
		declaredAncestry := map[string]struct{}{}
		for _, s := range c.Sources {
			if s.Locator != "" {
				declaredLocators[s.Locator] = struct{}{}
			}
			for _, ar := range s.AncestryRoots {
				if ar != "" {
					declaredAncestry[ar] = struct{}{}
				}
			}
		}
		declaredContradictionIDs := map[string]struct{}{}
		for _, ct := range c.Contradictions {
			if ct.ContradictionID != "" {
				declaredContradictionIDs[ct.ContradictionID] = struct{}{}
			}
		}
		declaredGapIDs := map[string]struct{}{}
		for _, g := range c.Gaps {
			if g.GapID != "" {
				declaredGapIDs[g.GapID] = struct{}{}
			}
		}
		for j := range c.Hazards {
			h := &c.Hazards[j]
			hpep := fmt.Sprintf("%s.hazards[%d]", cep, j)
			// Leg 1: hazard_ref.
			if h.HazardRef == "" {
				errs = append(errs, hpep+": empty hazard_ref")
			}
			// Leg 2: symptom_refs (>=1; survival chain starts here).
			if len(h.SymptomRefs) == 0 {
				errs = append(errs, hpep+": hazard has no symptom_refs (survival chain starts at hazard_ref -> symptom_refs)")
			}
			// Leg 3: source_refs (>=1 mandatory; each resolves to a declared
			// source locator on this conclusion). A hazard with no source
			// locators is rootless and cannot survive.
			if len(h.SourceLocators) == 0 {
				errs = append(errs, hpep+": hazard has no source_locators (survival chain requires source_refs -> ancestry)")
			}
			for _, loc := range h.SourceLocators {
				if loc == "" {
					errs = append(errs, hpep+": empty source_locator (fabricated/locator-free evidence invalid)")
				} else if _, ok := declaredLocators[loc]; !ok {
					errs = append(errs, fmt.Sprintf("%s: source_locator %q does not resolve to any declared source on this conclusion", hpep, loc))
				}
			}
			// Leg 4: ancestry (a non-empty hazard ancestry root must ALWAYS
			// resolve to a declared source's ancestry root — there is no
			// "no declared ancestry => anything passes" escape. Survival is
			// NOT inferred from fabricated ancestry.)
			for _, ar := range h.AncestryRoots {
				if ar == "" {
					errs = append(errs, hpep+": empty ancestry_root")
				} else if _, ok := declaredAncestry[ar]; !ok {
					errs = append(errs, fmt.Sprintf("%s: ancestry_root %q does not resolve to any declared source ancestry on this conclusion", hpep, ar))
				}
			}
			// Leg 5: contradiction/gap (optional, but if present must resolve to
			// a declared contradiction_id / gap_id on this conclusion).
			if h.ContradictionRef != "" {
				if _, ok := declaredContradictionIDs[h.ContradictionRef]; !ok {
					errs = append(errs, fmt.Sprintf("%s: contradiction_ref %q does not resolve to any declared contradiction_id on this conclusion", hpep, h.ContradictionRef))
				}
			}
			if h.GapRef != "" {
				if _, ok := declaredGapIDs[h.GapRef]; !ok {
					errs = append(errs, fmt.Sprintf("%s: gap_ref %q does not resolve to any declared gap_id on this conclusion", hpep, h.GapRef))
				}
			}
		}
	}
	return errs
}

func validateR3Summary(pe string, r3 *F1R3ForkSummary) []string {
	if r3 == nil {
		return nil
	}
	var errs []string
	if _, ok := f1ValidR3Dispositions[r3.Disposition]; !ok {
		errs = append(errs, fmt.Sprintf("%s.r3: unknown disposition %q (want one of %s)", pe, r3.Disposition, f1SortedKeys(f1ValidR3Dispositions)))
	}
	oids := map[string]int{}
	for i := range r3.Options {
		o := &r3.Options[i]
		oep := fmt.Sprintf("%s.r3.options[%d]", pe, i)
		if o.OptionID == "" {
			errs = append(errs, oep+": empty option_id")
		} else if prev, dup := oids[o.OptionID]; dup {
			errs = append(errs, fmt.Sprintf("%s: duplicate option_id %q (also at options[%d])", oep, o.OptionID, prev))
		} else {
			oids[o.OptionID] = i
		}
		if _, ok := f1ValidR3Modes[o.Mode]; !ok {
			errs = append(errs, fmt.Sprintf("%s: unknown mode %q (want one of %s)", oep, o.Mode, f1SortedKeys(f1ValidR3Modes)))
		}
		if o.Mechanism == "" {
			errs = append(errs, oep+": empty mechanism")
		}
		if dup := firstDuplicate(o.SupportRefs); dup != "" {
			errs = append(errs, fmt.Sprintf("%s: duplicate support_ref %q", oep, dup))
		}
		if dup := firstDuplicate(o.CounterEvidenceProbeRefs); dup != "" {
			errs = append(errs, fmt.Sprintf("%s: duplicate counter_evidence_probe_ref %q", oep, dup))
		}
	}
	// Fork-completeness (trigger ⇒ both options + material difference +
	// P-a coverage + R1 basis). Shared with the R3 transition gate so a
	// committed projection and a transition decision see the same rule.
	errs = append(errs, validateR3ForkCompleteness(pe+".r3", r3)...)
	// Selection record (when disposition==selected): shared with the gate so a
	// committed projection cannot carry selected-without-selection.
	if r3.Disposition == F1R3DispositionSelected {
		errs = append(errs, validateR3Selection(pe+".r3", r3)...)
	}
	return errs
}

func validatePASummary(pe string, pa *F1PAProbeSummary) []string {
	if pa == nil {
		return nil
	}
	var errs []string
	pids := map[string]int{}
	for i := range pa.Probes {
		p := &pa.Probes[i]
		ppep := fmt.Sprintf("%s.pa.probes[%d]", pe, i)
		if p.ProbeID == "" {
			errs = append(errs, ppep+": empty probe_id")
		} else if prev, dup := pids[p.ProbeID]; dup {
			errs = append(errs, fmt.Sprintf("%s: duplicate probe_id %q (also at probes[%d])", ppep, p.ProbeID, prev))
		} else {
			pids[p.ProbeID] = i
		}
		if _, ok := f1ValidPAResults[p.Result]; !ok {
			errs = append(errs, fmt.Sprintf("%s: unknown result %q (want one of %s)", ppep, p.Result, f1SortedKeys(f1ValidPAResults)))
		}
		if p.TargetRef == "" {
			errs = append(errs, ppep+": empty target_ref")
		}
		if dup := firstDuplicate(p.CheckedScope); dup != "" {
			errs = append(errs, fmt.Sprintf("%s: duplicate checked_scope %q", ppep, dup))
		}
		if dup := firstDuplicate(p.EvidenceRefs); dup != "" {
			errs = append(errs, fmt.Sprintf("%s: duplicate evidence_ref %q", ppep, dup))
		}
	}
	// Per-result requirements (memo L201-206): found needs real refs;
	// not_found_in_checked_scope needs method+scope (BOUNDED, never global);
	// unavailable needs a limitation; not_run needs nothing (but cannot
	// satisfy coverage — enforced by validateEnvelopePACoverage / the gate).
	errs = append(errs, validatePAProbeRequirements(pe, pa)...)
	return errs
}

// validatePAProbeRequirements enforces the per-result evidence-honesty rules.
// It is factored out so BOTH the envelope validator (validatePASummary) AND
// the standalone coverage gate (f1_pa_gate.go) apply the same rules — the gate
// cannot assume the envelope validator ran upstream (defense-in-depth).
//
// Structural, not truth: a well-formed but fabricated locator passes (proving
// truth is the federated verifier's job, memo L276). An empty/blank locator
// does NOT count as evidence.
func validatePAProbeRequirements(pe string, pa *F1PAProbeSummary) []string {
	if pa == nil {
		return nil
	}
	var errs []string
	for i := range pa.Probes {
		p := &pa.Probes[i]
		ppep := fmt.Sprintf("%s.pa.probes[%d]", pe, i)
		switch p.Result {
		case F1PAResultFound:
			// found REQUIRES >=1 non-empty (trimmed) real evidence locator.
			if countNonEmpty(p.EvidenceRefs) == 0 {
				errs = append(errs, ppep+": result=found requires >=1 non-empty evidence_ref (real locator; fabricated/locator-free evidence is invalid under any result)")
			}
		case F1PAResultNotFoundInCheckedScope:
			// BOUNDED absence: requires method + checked scope. This is NEVER
			// global absence — the result is scoped to CheckedScope, and the
			// validator/gate must not convert it to a global-absence claim.
			if strings.TrimSpace(p.Method) == "" {
				errs = append(errs, ppep+": result=not_found_in_checked_scope requires a non-empty method")
			}
			if countNonEmpty(p.CheckedScope) == 0 {
				errs = append(errs, ppep+": result=not_found_in_checked_scope requires >=1 non-empty checked_scope (BOUNDED absence, never global)")
			}
		case F1PAResultUnavailable:
			// unavailable REQUIRES an explicit limitation. Structurally valid
			// coverage, but cannot support proven (memo L309).
			if strings.TrimSpace(p.Limitation) == "" {
				errs = append(errs, ppep+": result=unavailable requires a non-empty limitation")
			}
		case F1PAResultNotRun:
			// not_run: no per-probe requirement, but does NOT satisfy coverage
			// (validateEnvelopePACoverage / the gate enforce that separately).
		}
	}
	return errs
}

// --- P-a target coverage ---------------------------------------------------

// paCoverageSatisfies reports whether a probe result satisfies the target-
// coverage requirement. Only the three bounded-result values satisfy
// coverage: found / not_found_in_checked_scope / unavailable (a probe was
// attempted and produced a bounded result). not_run does NOT (memo L206).
// unavailable satisfies coverage structurally but cannot support proven
// (memo L309) — the high-risk-release-seam policy that would ALSO block on
// unavailable is operator-owned (open-question #5) and is NOT enforced here.
//
// This is an EXPLICIT ALLOWLIST (not `!= not_run`) so that an UNKNOWN result
// string is treated as non-coverage-satisfying rather than silently accepted
// — F1PAResult is a closed enum, and the coverage predicate must not widen it.
// The standalone gate (f1_pa_gate.go) additionally rejects unknown results
// explicitly (defense-in-depth); this allowlist is the backstop.
func paCoverageSatisfies(result string) bool {
	switch result {
	case F1PAResultFound, F1PAResultNotFoundInCheckedScope, F1PAResultUnavailable:
		return true
	}
	return false
}

// paTargetSet computes the set of targets that require P-a coverage: every
// declared R1 conclusion ID (all declared conclusions are treated as material
// — the producer decides materiality by what it declares) + every declared R3
// option ID (when an R3 summary is present). Sorted for deterministic output.
func paTargetSet(r1 *F1R1JoinSummary, r3 *F1R3ForkSummary) []string {
	seen := map[string]struct{}{}
	var targets []string
	add := func(id string) {
		if id == "" {
			return
		}
		if _, dup := seen[id]; dup {
			return
		}
		seen[id] = struct{}{}
		targets = append(targets, id)
	}
	if r1 != nil {
		for _, c := range r1.Conclusions {
			add(c.ConclusionID)
		}
	}
	if r3 != nil {
		for _, o := range r3.Options {
			add(o.OptionID)
		}
	}
	sort.Strings(targets)
	return targets
}

// validatePACoverageSet checks that every target in targets has >=1 coverage-
// satisfying probe in pa (result != not_run). Shared between the envelope
// validator and the standalone gate so both apply the identical rule.
func validatePACoverageSet(prefix string, targets []string, pa *F1PAProbeSummary) []string {
	covered := map[string]struct{}{}
	if pa != nil {
		for _, p := range pa.Probes {
			if paCoverageSatisfies(p.Result) && p.TargetRef != "" {
				covered[p.TargetRef] = struct{}{}
			}
		}
	}
	var errs []string
	for _, t := range targets {
		if _, ok := covered[t]; !ok {
			errs = append(errs, fmt.Sprintf("%s: target %q has no coverage-satisfying probe (needs >=1 probe with result in {found, not_found_in_checked_scope, unavailable}; not_run does not satisfy coverage)", prefix, t))
		}
	}
	return errs
}

// validateEnvelopePACoverage is the envelope-level instance of the require-P-
// a-target-coverage gate (memo L274). Coverage is required only when the
// envelope is applicable (applicability==required); a not_triggered envelope
// is N/A as a whole and has no coverage requirement. When applicable, every
// material R1 conclusion + every declared R3 option must be covered.
func validateEnvelopePACoverage(env *F1SynthesisEnvelope) []string {
	if env.Applicability != F1ApplicabilityRequired {
		return nil
	}
	var r1 *F1R1JoinSummary
	var r3 *F1R3ForkSummary
	var pa *F1PAProbeSummary
	for i := range env.Entries {
		e := &env.Entries[i]
		switch e.Family {
		case F1FamilyR1CrossLaneJoin:
			r1 = e.R1
		case F1FamilyR3RedesignFork:
			r3 = e.R3
		case F1FamilyPACounterEvidence:
			pa = e.PA
		}
	}
	targets := paTargetSet(r1, r3)
	if len(targets) == 0 {
		return nil // no material conclusions/options -> no coverage required
	}
	return validatePACoverageSet("pa-coverage", targets, pa)
}

// countNonEmpty returns the number of non-empty (trimmed) strings in s.
// Empty/blank evidence refs and checked-scope entries do not count.
func countNonEmpty(s []string) int {
	n := 0
	for _, v := range s {
		if strings.TrimSpace(v) != "" {
			n++
		}
	}
	return n
}

// --- cross-reference resolution --------------------------------------------

// resolveF1CrossRefs verifies that inter-family references resolve to a
// declared ID within the envelope:
//   - R3 option SupportRefs            -> R1 conclusion IDs
//   - R3 option CounterEvidenceProbeRefs -> P-a probe IDs
//   - P-a probe TargetRef              -> R1 conclusion ID OR R3 option ID
//
// A dangling reference is a structural inconsistency (the families disagree
// about what exists). Resolution is structural; it does not assess whether a
// link is semantically meaningful. Indexing and resolution are FAMILY-GATED:
// only an r1 entry's R1 summary contributes conclusion IDs, only an r3 entry's
// R3 summary contributes option IDs, and only a pa entry's PA summary
// contributes probe IDs. This is defense-in-depth: the family↔summary binding
// check already rejects foreign summaries, so a foreign summary cannot reach
// this resolver — but gating here means the resolver stays correct by
// construction even if that check were ever weakened.
func resolveF1CrossRefs(env *F1SynthesisEnvelope) []string {
	r1Conclusions := map[string]struct{}{}
	r3Options := map[string]struct{}{}
	paProbes := map[string]struct{}{}
	for i := range env.Entries {
		e := &env.Entries[i]
		switch e.Family {
		case F1FamilyR1CrossLaneJoin:
			if e.R1 != nil {
				for j := range e.R1.Conclusions {
					r1Conclusions[e.R1.Conclusions[j].ConclusionID] = struct{}{}
				}
			}
		case F1FamilyR3RedesignFork:
			if e.R3 != nil {
				for j := range e.R3.Options {
					r3Options[e.R3.Options[j].OptionID] = struct{}{}
				}
			}
		case F1FamilyPACounterEvidence:
			if e.PA != nil {
				for j := range e.PA.Probes {
					paProbes[e.PA.Probes[j].ProbeID] = struct{}{}
				}
			}
		}
	}
	var errs []string
	for i := range env.Entries {
		e := &env.Entries[i]
		switch e.Family {
		case F1FamilyR3RedesignFork:
			if e.R3 == nil {
				continue
			}
			for j := range e.R3.Options {
				o := &e.R3.Options[j]
				oep := fmt.Sprintf("entries[%d].r3.options[%d]", i, j)
				for _, ref := range o.SupportRefs {
					if _, ok := r1Conclusions[ref]; !ok {
						errs = append(errs, fmt.Sprintf("%s: support_ref %q does not resolve to any r1 conclusion_id", oep, ref))
					}
				}
				for _, ref := range o.CounterEvidenceProbeRefs {
					if _, ok := paProbes[ref]; !ok {
						errs = append(errs, fmt.Sprintf("%s: counter_evidence_probe_ref %q does not resolve to any pa probe_id", oep, ref))
					}
				}
			}
		case F1FamilyPACounterEvidence:
			if e.PA == nil {
				continue
			}
			for j := range e.PA.Probes {
				p := &e.PA.Probes[j]
				ppep := fmt.Sprintf("entries[%d].pa.probes[%d]", i, j)
				_, isR1 := r1Conclusions[p.TargetRef]
				_, isR3 := r3Options[p.TargetRef]
				if !isR1 && !isR3 {
					errs = append(errs, fmt.Sprintf("%s: target_ref %q does not resolve to any r1 conclusion_id or r3 option_id", ppep, p.TargetRef))
				}
			}
		case F1FamilyR1CrossLaneJoin:
			// R1 hazard survival chain — cross-family leg: each hazard's
			// consuming R3 option IDs must resolve to a declared r3 option_id,
			// and each consuming P-a probe ID must resolve to a declared pa
			// probe_id. The within-conclusion leg (source locators resolve to
			// the conclusion's own sources) is checked in validateR1Summary.
			if e.R1 == nil {
				continue
			}
			for cj := range e.R1.Conclusions {
				c := &e.R1.Conclusions[cj]
				for hj := range c.Hazards {
					h := &c.Hazards[hj]
					hpep := fmt.Sprintf("entries[%d].r1.conclusions[%d].hazards[%d]", i, cj, hj)
					for _, ref := range h.ConsumingR3OptionIDs {
						if _, ok := r3Options[ref]; !ok {
							errs = append(errs, fmt.Sprintf("%s: consuming_r3_option_id %q does not resolve to any r3 option_id", hpep, ref))
						}
					}
					for _, ref := range h.ConsumingPAProbeIDs {
						if _, ok := paProbes[ref]; !ok {
							errs = append(errs, fmt.Sprintf("%s: consuming_pa_probe_id %q does not resolve to any pa probe_id", hpep, ref))
						}
					}
				}
			}
		}
	}
	return errs
}

// --- small helpers ---------------------------------------------------------

// firstDuplicate returns the first string appearing more than once in s, or
// "" if all entries are distinct (or s is empty).
func firstDuplicate(s []string) string {
	seen := map[string]struct{}{}
	for _, v := range s {
		if _, ok := seen[v]; ok {
			return v
		}
		seen[v] = struct{}{}
	}
	return ""
}

// f1SortedKeys returns the keys of m in sorted order, formatted as a
// space-separated list for inclusion in error messages. F1-scoped name to
// avoid colliding with the package's existing sortedKeys helper.
func f1SortedKeys(m map[string]struct{}) string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	out := ""
	for i, k := range ks {
		if i > 0 {
			out += " "
		}
		out += k
	}
	return out
}
