package cli

// f1_pa_dedupe_test.go — P1-CLI-002 regression: the P-a producer must dedupe
// caller-supplied EvidenceRefs and CheckedScope on COPIES, so that a caller
// passing duplicate entries produces output that passes validatePASummary's
// firstDuplicate check. The producer must NOT mutate the caller's slices
// (purity), and probe IDs must remain stable/deterministic (PA-P1, PA-P2, ...).
//
// Scope: producer-purity hole only. Does NOT exercise the per-result honesty
// matrix or the closed-enum coverage semantics (those are pinned by
// f1_pa_test.go and stay unchanged).

import (
	"reflect"
	"strings"
	"testing"
)

// TestGeneratePAProbes_DedupesDuplicateEvidenceRefsAndCheckedScope is the
// primary regression: a caller passing duplicate EvidenceRefs and CheckedScope
// entries must get producer output that PASSES validatePASummary (no
// firstDuplicate error). Before P1-CLI-002 this failed; now it must pass.
func TestGeneratePAProbes_DedupesDuplicateEvidenceRefsAndCheckedScope(t *testing.T) {
	inputs := []PAProbeInput{
		{
			TargetRef: "R1C1",
			Result:    F1PAResultFound,
			// Duplicates across non-adjacent positions and adjacent positions.
			EvidenceRefs: []string{"ref-a", "ref-b", "ref-a", "ref-a", "ref-c"},
		},
		{
			TargetRef: "R1C2",
			Result:    F1PAResultNotFoundInCheckedScope,
			Method:    "grep",
			// Duplicates in CheckedScope (sort + dedup path).
			CheckedScope: []string{"src/z/", "src/a/", "src/z/", "src/a/", "src/m/"},
		},
	}

	summary, perrs := GeneratePAProbes(inputs)
	if len(perrs) != 0 {
		t.Fatalf("producer returned errors for duplicate-bearing but well-formed input: %v", perrs)
	}
	// The headline assertion: the producer output passes the validator
	// (firstDuplicate check on both CheckedScope and EvidenceRefs).
	if verrs := validatePASummary("e", summary); len(verrs) != 0 {
		t.Fatalf("deduped producer output must pass validatePASummary; got:\n  %s", strings.Join(verrs, "\n  "))
	}

	// Defensive: explicitly confirm NO duplicate remains in any probe slice.
	for i, p := range summary.Probes {
		if d := firstDuplicate(p.CheckedScope); d != "" {
			t.Errorf("probes[%d] CheckedScope still has duplicate %q after dedup: %v", i, d, p.CheckedScope)
		}
		if d := firstDuplicate(p.EvidenceRefs); d != "" {
			t.Errorf("probes[%d] EvidenceRefs still has duplicate %q after dedup: %v", i, d, p.EvidenceRefs)
		}
	}
}

// TestGeneratePAProbes_DedupeDoesNotMutateCaller asserts the producer-purity
// half of P1-CLI-002: dedup happens on COPIES, so a caller passing duplicates
// gets its slices back unmodified (length, order, content). This is the
// dedup-specific analog of TestGeneratePAProbes_DoesNotMutateInputs.
func TestGeneratePAProbes_DedupeDoesNotMutateCaller(t *testing.T) {
	inputs := []PAProbeInput{
		{
			TargetRef:    "R1C1",
			Result:       F1PAResultFound,
			EvidenceRefs: []string{"ref-a", "ref-a", "ref-b"},
		},
	}
	// Snapshot the caller's slice (independent backing array).
	wantRefs := append([]string{}, inputs[0].EvidenceRefs...)
	_, _ = GeneratePAProbes(inputs)
	if got := inputs[0].EvidenceRefs; !reflect.DeepEqual(got, wantRefs) {
		t.Fatalf("dedup mutated caller EvidenceRefs: got %v want %v (must dedupe on a copy)", got, wantRefs)
	}

	inputs2 := []PAProbeInput{
		{
			TargetRef:    "R1C2",
			Result:       F1PAResultNotFoundInCheckedScope,
			Method:       "grep",
			CheckedScope: []string{"src/z/", "src/a/", "src/z/"},
		},
	}
	wantScope := append([]string{}, inputs2[0].CheckedScope...)
	_, _ = GeneratePAProbes(inputs2)
	if got := inputs2[0].CheckedScope; !reflect.DeepEqual(got, wantScope) {
		t.Fatalf("dedup mutated caller CheckedScope: got %v want %v (must dedupe on a copy)", got, wantScope)
	}
}

// TestGeneratePAProbes_DedupeKeepsStableProbeIDs pins that dedup does not
// perturb the deterministic PA-P1/PA-P2/... probe-ID assignment. Probe IDs are
// assigned by sorted-input order; dedup happens per-probe on slice fields and
// must not change which inputs become probes or their relative order.
func TestGeneratePAProbes_DedupeKeepsStableProbeIDs(t *testing.T) {
	// Inputs deliberately out of TargetRef order so the stable sort is
	// exercised; duplicates inside the slices must not re-shuffle probe IDs.
	inputs := []PAProbeInput{
		{TargetRef: "Z-tgt", Result: F1PAResultFound, EvidenceRefs: []string{"r1", "r1", "r2"}},
		{TargetRef: "A-tgt", Result: F1PAResultUnavailable, Limitation: "lim", EvidenceRefs: []string{"r1", "r1"}},
	}
	summary, perrs := GeneratePAProbes(inputs)
	if len(perrs) != 0 {
		t.Fatalf("producer returned errors: %v", perrs)
	}
	wantIDs := []string{"PA-P1", "PA-P2"}
	if len(summary.Probes) != len(wantIDs) {
		t.Fatalf("expected %d probes, got %d", len(wantIDs), len(summary.Probes))
	}
	// Stable sort orders A-tgt before Z-tgt regardless of caller order.
	for i, want := range wantIDs {
		if got := summary.Probes[i].ProbeID; got != want {
			t.Errorf("probes[%d].ProbeID = %q, want %q (stable IDs perturbed by dedup)", i, got, want)
		}
	}
	if got, want := summary.Probes[0].TargetRef, "A-tgt"; got != want {
		t.Errorf("probes[0].TargetRef = %q, want %q (stable sort perturbed by dedup)", got, want)
	}
	if got, want := summary.Probes[1].TargetRef, "Z-tgt"; got != want {
		t.Errorf("probes[1].TargetRef = %q, want %q (stable sort perturbed by dedup)", got, want)
	}
}

// TestGeneratePAProbes_DedupePreservesEvidenceRefOrder pins that EvidenceRefs
// dedup is ORDER-PRESERVING (caller order is meaningful for evidence and the
// producer must not reorder it, unlike CheckedScope which is sorted). This
// guards against a future change that swaps EvidenceRefs to the sorted path.
func TestGeneratePAProbes_DedupePreservesEvidenceRefOrder(t *testing.T) {
	inputs := []PAProbeInput{
		{
			TargetRef: "R1C1",
			Result:    F1PAResultFound,
			// Deliberately unsorted + duplicates: dedup must drop the dupes
			// while keeping the FIRST occurrence in caller order.
			EvidenceRefs: []string{"zebra", "apple", "zebra", "mango", "apple"},
		},
	}
	summary, perrs := GeneratePAProbes(inputs)
	if len(perrs) != 0 {
		t.Fatalf("producer returned errors: %v", perrs)
	}
	if len(summary.Probes) != 1 {
		t.Fatalf("expected 1 probe, got %d", len(summary.Probes))
	}
	got := summary.Probes[0].EvidenceRefs
	want := []string{"zebra", "apple", "mango"} // first-occurrence order, NOT sorted
	if len(got) != len(want) {
		t.Fatalf("EvidenceRefs = %v, want %v (length mismatch)", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("EvidenceRefs[%d] = %q, want %q (dedup must preserve caller order, not sort)", i, got[i], want[i])
		}
	}
}

// TestGeneratePAProbes_DedupeHandlesNilAndEmptySlices pins the nil-vs-empty
// preservation: a nil caller slice yields a nil/empty producer slice (no
// spurious allocation, no shape change vs. the pre-dedup producer).
func TestGeneratePAProbes_DedupeHandlesNilAndEmptySlices(t *testing.T) {
	t.Run("nil evidence refs on found", func(t *testing.T) {
		// found with nil refs is a producer ERROR (requires >=1 non-empty
		// ref); verify dedup path does not mask that.
		_, perrs := GeneratePAProbes([]PAProbeInput{
			{TargetRef: "R1C1", Result: F1PAResultFound, EvidenceRefs: nil},
		})
		if len(perrs) == 0 {
			t.Fatal("found with nil EvidenceRefs must still produce a requirement error after dedup")
		}
	})
	t.Run("empty evidence refs on not_run", func(t *testing.T) {
		// not_run has no per-input requirement; nil/empty refs are valid.
		summary, perrs := GeneratePAProbes([]PAProbeInput{
			{TargetRef: "R1C1", Result: F1PAResultNotRun, EvidenceRefs: nil},
		})
		if len(perrs) != 0 {
			t.Fatalf("not_run with nil refs must not error: %v", perrs)
		}
		if len(summary.Probes) != 1 {
			t.Fatalf("expected 1 probe, got %d", len(summary.Probes))
		}
		// No panic, no duplicate; the slice is empty (nil or len-0).
		if d := firstDuplicate(summary.Probes[0].EvidenceRefs); d != "" {
			t.Errorf("nil EvidenceRefs produced duplicate %q", d)
		}
	})
	t.Run("empty checked scope on not_run", func(t *testing.T) {
		summary, perrs := GeneratePAProbes([]PAProbeInput{
			{TargetRef: "R1C1", Result: F1PAResultNotRun, CheckedScope: nil},
		})
		if len(perrs) != 0 {
			t.Fatalf("not_run with nil scope must not error: %v", perrs)
		}
		if d := firstDuplicate(summary.Probes[0].CheckedScope); d != "" {
			t.Errorf("nil CheckedScope produced duplicate %q", d)
		}
	})
}
