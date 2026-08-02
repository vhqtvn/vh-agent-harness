package cli

// f1_pa.go — the P-a counter-evidence PRODUCER (the INFORMS half of memo
// L273: "P-a probe generation = INFORMS (F1 producer)"). Pure and deterministic:
// given explicit per-target probe inputs, it assembles a well-formed
// F1PAProbeSummary with stable probe IDs, normalized fields, and deep-copied
// slices (the producer never aliases caller memory).
//
// The producer GENERATES probes; it does NOT enforce coverage. Coverage (every
// material target has >=1 coverage-satisfying probe) is the GATE-SHAPED
// CONVERSION (memo L274, require-P-a-target-coverage) implemented in
// f1_pa_gate.go. The producer informs; the gate acts.
//
// The producer validates the per-input result-consistency (a found input with
// no evidence refs is a producer error) so that well-formed inputs yield output
// that passes validatePASummary — the producer↔validator property guarantee
// (mirrors TestJoinR1_ProducerOutputPassesValidator).
//
// OPERATOR-OWNED BOUNDARY (memo open-question #5, L307-311): the baseline
// rules ARE decided here — found needs refs, not_found_in_checked_scope needs
// method+scope, unavailable needs a limitation, not_run cannot satisfy
// coverage. The NOT-decided part is whether high-risk release seams ALSO block
// on `unavailable`; that extension is deferred and is NOT implemented here.

import (
	"fmt"
	"sort"
	"strings"
)

// PAProbeInput is the caller-supplied input for one counter-evidence probe.
// The producer takes these as typed values (no narrative inference), mirroring
// how GenerateR3Fork takes RepairIntent / StructuralReviewOutcome.
type PAProbeInput struct {
	TargetRef             string
	FalsificationQuestion string
	Result                string
	Method                string
	CheckedScope          []string
	EvidenceRefs          []string
	Limitation            string
	WeakestClaim          string
	Confidence            string
}

// GeneratePAProbes assembles a well-formed F1PAProbeSummary from explicit
// per-target inputs. It is PURE: it never mutates the caller's slices — every
// slice field is copied and deduped on that copy (CheckedScope via
// paSortedDedupedScope; EvidenceRefs via paDedupedRefs). It assigns stable
// probe IDs (PA-P1, PA-P2, ... in deterministic input order after a stable
// sort), normalizes whitespace on scalar fields, validates the result enum,
// and returns producer errors for inputs whose result-specific requirements
// are not met (so a well-formed input set produces output that passes
// validatePASummary).
func GeneratePAProbes(inputs []PAProbeInput) (*F1PAProbeSummary, []string) {
	var producerErrs []string

	// Stable order: sort a copy of input indices by TargetRef then by a
	// result-rank so output is deterministic regardless of caller order. We
	// sort indices (not the caller's slice) to preserve purity.
	idx := make([]int, len(inputs))
	for i := range idx {
		idx[i] = i
	}
	sort.SliceStable(idx, func(a, b int) bool {
		ia, ib := inputs[idx[a]], inputs[idx[b]]
		if ia.TargetRef != ib.TargetRef {
			return ia.TargetRef < ib.TargetRef
		}
		return paResultRank(ia.Result) < paResultRank(ib.Result)
	})

	probes := make([]F1PAProbe, 0, len(inputs))
	seenIDs := map[string]struct{}{}
	for n, i := range idx {
		in := inputs[i]
		if strings.TrimSpace(in.TargetRef) == "" {
			producerErrs = append(producerErrs, fmt.Sprintf("inputs[%d]: empty target_ref", i))
			continue
		}
		if _, ok := f1ValidPAResults[in.Result]; !ok {
			producerErrs = append(producerErrs, fmt.Sprintf("inputs[%d] (target %q): unknown result %q (want one of %s)", i, in.TargetRef, in.Result, f1SortedKeys(f1ValidPAResults)))
			continue
		}
		// Per-input result-consistency: produce output that passes
		// validatePAProbeRequirements. An inconsistent input is a producer
		// error (do not silently emit validator-failing output).
		if rerr := paInputRequirementErrors(i, in); len(rerr) > 0 {
			producerErrs = append(producerErrs, rerr...)
			continue
		}
		probe := F1PAProbe{
			ProbeID:               paProbeID(n + 1),
			TargetRef:             strings.TrimSpace(in.TargetRef),
			FalsificationQuestion: strings.TrimSpace(in.FalsificationQuestion),
			Result:                in.Result,
			Method:                strings.TrimSpace(in.Method),
			CheckedScope:          paSortedDedupedScope(in.CheckedScope),
			EvidenceRefs:          paDedupedRefs(in.EvidenceRefs),
			Limitation:            strings.TrimSpace(in.Limitation),
			WeakestClaim:          strings.TrimSpace(in.WeakestClaim),
			Confidence:            strings.TrimSpace(in.Confidence),
		}
		if _, dup := seenIDs[probe.ProbeID]; dup {
			producerErrs = append(producerErrs, fmt.Sprintf("internal: duplicate generated probe_id %q", probe.ProbeID))
			continue
		}
		seenIDs[probe.ProbeID] = struct{}{}
		probes = append(probes, probe)
	}

	summary := &F1PAProbeSummary{Probes: probes}
	return summary, producerErrs
}

// paInputRequirementErrors returns producer errors for an input whose
// result-specific fields do not meet the per-result contract. This mirrors
// validatePAProbeRequirements so the producer emits validator-consistent
// output (the producer↔validator property guarantee).
func paInputRequirementErrors(inputIdx int, in PAProbeInput) []string {
	var errs []string
	pi := fmt.Sprintf("inputs[%d] (target %q)", inputIdx, in.TargetRef)
	switch in.Result {
	case F1PAResultFound:
		if countNonEmpty(in.EvidenceRefs) == 0 {
			errs = append(errs, pi+": result=found requires >=1 non-empty evidence_ref")
		}
	case F1PAResultNotFoundInCheckedScope:
		if strings.TrimSpace(in.Method) == "" {
			errs = append(errs, pi+": result=not_found_in_checked_scope requires a non-empty method")
		}
		if countNonEmpty(in.CheckedScope) == 0 {
			errs = append(errs, pi+": result=not_found_in_checked_scope requires >=1 non-empty checked_scope")
		}
	case F1PAResultUnavailable:
		if strings.TrimSpace(in.Limitation) == "" {
			errs = append(errs, pi+": result=unavailable requires a non-empty limitation")
		}
	case F1PAResultNotRun:
		// no per-input requirement
	}
	return errs
}

// paProbeID returns a stable, sanitized probe ID from a 1-based sequence
// number (PA-P1, PA-P2, ...).
func paProbeID(n int) string {
	return fmt.Sprintf("PA-P%d", n)
}

// paResultRank gives a deterministic secondary sort key for probes sharing a
// target (found < not_found_in_checked_scope < unavailable < not_run).
func paResultRank(result string) int {
	switch result {
	case F1PAResultFound:
		return 0
	case F1PAResultNotFoundInCheckedScope:
		return 1
	case F1PAResultUnavailable:
		return 2
	case F1PAResultNotRun:
		return 3
	}
	return 4
}

// paSortedDedupedScope returns a sorted, deduplicated copy of s for the
// producer's CheckedScope field. The caller's slice is never mutated (pure
// copy); dedup runs on the copy. This closes the producer-purity gap where a
// caller passing duplicate scope entries would emit output that fails
// validatePASummary's firstDuplicate check (memo P1-CLI-002).
//
// Dedup is on EXACT string values: firstDuplicate operates on raw (untrimmed)
// values, so the producer must dedupe on the same basis.
//
// Nil-vs-empty behavior is preserved: a nil input returns a non-nil empty
// slice (matching the prior sortedCopyStrings semantics so JSON canonical
// bytes and downstream consumers see no shape change).
func paSortedDedupedScope(s []string) []string {
	out := make([]string, len(s))
	copy(out, s)
	sort.Strings(out)
	return paCompactAdjacentDuplicates(out)
}

// paDedupedRefs returns an order-preserving, deduplicated copy of s for the
// producer's EvidenceRefs field. Caller order is preserved (evidence refs may
// carry caller-meaningful ordering that the producer must not reorder). The
// caller's slice is never mutated (pure copy); dedup runs on the copy.
//
// Dedup is on EXACT string values (matches firstDuplicate's raw comparison).
// Nil input returns nil (matches the prior copyStrings semantics).
func paDedupedRefs(s []string) []string {
	if s == nil {
		return nil
	}
	out := make([]string, 0, len(s))
	seen := map[string]struct{}{}
	for _, v := range s {
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

// paCompactAdjacentDuplicates compacts a sorted slice by dropping adjacent
// duplicates in place. Used after sort.Strings for the CheckedScope path so
// dedup is stable against the sorted order without a second allocation.
func paCompactAdjacentDuplicates(sorted []string) []string {
	if len(sorted) == 0 {
		return sorted
	}
	w := 1
	for r := 1; r < len(sorted); r++ {
		if sorted[r] != sorted[r-1] {
			sorted[w] = sorted[r]
			w++
		}
	}
	return sorted[:w]
}
