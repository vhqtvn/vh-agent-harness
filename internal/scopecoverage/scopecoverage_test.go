package scopecoverage

import (
	"reflect"
	"testing"
)

// These tests implement the F4-A validation plan from
// researches/decisions/2026-07-28-success-report-integrity-and-working-tree-stewardship.md:
//
//  1. fixture declared {a,b} vs reported {a} → incomplete / missing-b
//  2. declared {a,b} vs {a,b examined} → pass (complete)
//  3. undeclared {c} → mismatch (extra)
//  4. fail-fast termination cannot produce complete approval while any declared
//     item lacks a terminal disposition
//  5. validator claims structural coverage only, not semantic examination
//  6. (build verification: go test ./..., go vet, gofmt — exercised by the
//     suite running green plus the repo-wide gates)

func item(p, c string) DeclaredScopeItem { return DeclaredScopeItem{Path: p, Concern: c} }

func dstat(id string, s DispositionStatus) CoverageDisposition {
	return CoverageDisposition{ItemID: id, Status: s}
}

// dexcl is a reasoned exclusion: StatusExcluded WITH a non-blank Reason, which
// is a VALID terminal disposition.
func dexcl(id, reason string) CoverageDisposition {
	return CoverageDisposition{ItemID: id, Status: StatusExcluded, Reason: reason}
}

// Fixture 1: declared {a,b} vs reported {a} → incomplete, missing b.
func TestValidate_Fixture1_MissingDeclaredItem(t *testing.T) {
	declared := []DeclaredScopeItem{
		item("a.go", ""),
		item("b.go", ""),
	}
	disp := []CoverageDisposition{
		dstat("a.go", StatusExamined),
		// b.go has no disposition
	}
	r := Validate(declared, disp, false)

	if r.Complete {
		t.Fatalf("expected incomplete, got Complete=true")
	}
	if !reflect.DeepEqual(r.Missing, []string{"b.go"}) {
		t.Fatalf("Missing = %v, want [b.go]", r.Missing)
	}
	if len(r.NonTerminal) != 0 {
		t.Fatalf("NonTerminal = %v, want empty", r.NonTerminal)
	}
}

// Fixture 2: declared {a,b} vs {a,b examined} → complete (pass).
func TestValidate_Fixture2_Complete(t *testing.T) {
	declared := []DeclaredScopeItem{
		item("a.go", ""),
		item("b.go", ""),
	}
	disp := []CoverageDisposition{
		dstat("a.go", StatusExamined),
		dstat("b.go", StatusExamined),
	}
	r := Validate(declared, disp, false)

	if !r.Complete {
		t.Fatalf("expected complete, got %+v", r)
	}
	if !reflect.DeepEqual(r.Missing, []string(nil)) {
		t.Fatalf("Missing = %v, want nil", r.Missing)
	}
	if !reflect.DeepEqual(r.NonTerminal, []string(nil)) {
		t.Fatalf("NonTerminal = %v, want nil", r.NonTerminal)
	}
	if !reflect.DeepEqual(r.Extra, []string(nil)) {
		t.Fatalf("Extra = %v, want nil", r.Extra)
	}
}

// Excluded-by-contract (WITH a reason) is also terminal: a complete report
// with one examined and one reasoned-excluded item.
func TestValidate_ExcludedIsTerminal(t *testing.T) {
	declared := []DeclaredScopeItem{
		item("a.go", ""),
		item("b.go", ""),
	}
	disp := []CoverageDisposition{
		dstat("a.go", StatusExamined),
		dexcl("b.go", "out of slice: generated/vendored"),
	}
	r := Validate(declared, disp, false)
	if !r.Complete {
		t.Fatalf("reasoned exclusion is terminal; expected complete, got %+v", r)
	}
	if len(r.UnexplainedExclusions) != 0 {
		t.Fatalf("UnexplainedExclusions = %v, want empty", r.UnexplainedExclusions)
	}
}

// F1 fix: an unexplained exclusion (StatusExcluded with blank Reason) is NOT a
// valid terminal disposition — "excluded by contract" implies a contract, so a
// reason-less exclusion is an omission in disguise. It blocks Complete and is
// reported in BOTH NonTerminal and UnexplainedExclusions.
func TestValidate_UnexplainedExclusionBlocksComplete(t *testing.T) {
	declared := []DeclaredScopeItem{item("a.go", "")}
	// empty reason
	r := Validate(declared, []CoverageDisposition{{ItemID: "a.go", Status: StatusExcluded, Reason: ""}}, false)
	if r.Complete {
		t.Fatalf("excluded with empty Reason must not be complete")
	}
	if !reflect.DeepEqual(r.UnexplainedExclusions, []string{"a.go"}) {
		t.Fatalf("UnexplainedExclusions = %v, want [a.go]", r.UnexplainedExclusions)
	}
	if !reflect.DeepEqual(r.NonTerminal, []string{"a.go"}) {
		t.Fatalf("NonTerminal = %v, want [a.go]", r.NonTerminal)
	}

	// whitespace-only reason is also unexplained
	r2 := Validate(declared, []CoverageDisposition{{ItemID: "a.go", Status: StatusExcluded, Reason: "   "}}, false)
	if r2.Complete {
		t.Fatalf("excluded with whitespace-only Reason must not be complete")
	}
	if !reflect.DeepEqual(r2.UnexplainedExclusions, []string{"a.go"}) {
		t.Fatalf("UnexplainedExclusions = %v, want [a.go]", r2.UnexplainedExclusions)
	}
}

// Fixture 3: undeclared {c} (disposition for an id not in declared scope) →
// mismatch / extra.
func TestValidate_Fixture3_ExtraOutOfScope(t *testing.T) {
	declared := []DeclaredScopeItem{
		item("a.go", ""),
	}
	disp := []CoverageDisposition{
		dstat("a.go", StatusExamined),
		dstat("c.go", StatusExamined), // not declared
	}
	r := Validate(declared, disp, false)

	if r.Complete {
		t.Fatalf("expected incomplete due to extra, got Complete=true")
	}
	if !reflect.DeepEqual(r.Extra, []string{"c.go"}) {
		t.Fatalf("Extra = %v, want [c.go]", r.Extra)
	}
}

// Fixture 4: fail-fast termination cannot produce complete approval while any
// declared item lacks a terminal disposition. fail-fast cut the review short so
// c.go never received a disposition.
func TestValidate_Fixture4_FailFastCannotCompleteWithGap(t *testing.T) {
	declared := []DeclaredScopeItem{
		item("a.go", ""),
		item("b.go", ""),
		item("c.go", ""),
	}
	disp := []CoverageDisposition{
		dstat("a.go", StatusExamined),
		dstat("b.go", StatusExamined),
		// c.go missing because fail-fast truncated the review
	}
	r := Validate(declared, disp, true)

	if r.Complete {
		t.Fatalf("fail-fast with a missing item must not be complete")
	}
	if !r.FailFastTerminated {
		t.Fatalf("FailFastTerminated flag should be echoed true")
	}
	if !reflect.DeepEqual(r.Missing, []string{"c.go"}) {
		t.Fatalf("Missing = %v, want [c.go]", r.Missing)
	}
}

// Fail-fast that nonetheless accounted for every declared item is structurally
// complete — fail-fast does not INDEPENDENTLY force incomplete. The flag is
// echoed so a consumer knows evidence was gathered under truncation.
func TestValidate_FailFastAloneDoesNotForceIncomplete(t *testing.T) {
	declared := []DeclaredScopeItem{
		item("a.go", ""),
		item("b.go", ""),
	}
	disp := []CoverageDisposition{
		dstat("a.go", StatusExamined),
		dstat("b.go", StatusExamined),
	}
	r := Validate(declared, disp, true)

	if !r.Complete {
		t.Fatalf("fail-fast with full coverage should be structurally complete; got %+v", r)
	}
	if !r.FailFastTerminated {
		t.Fatalf("FailFastTerminated should be echoed true")
	}
}

// Non-terminal dispositions (not_examined, blocked) block completeness even
// when present (they are not "missing" — they are present-but-incomplete).
func TestValidate_NonTerminalBlocksComplete(t *testing.T) {
	cases := []struct {
		name   string
		status DispositionStatus
	}{
		{"not_examined", StatusNotExamined},
		{"blocked", StatusBlocked},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			declared := []DeclaredScopeItem{item("a.go", "")}
			disp := []CoverageDisposition{dstat("a.go", tc.status)}
			r := Validate(declared, disp, false)
			if r.Complete {
				t.Fatalf("non-terminal %s must not be complete", tc.status)
			}
			if !reflect.DeepEqual(r.NonTerminal, []string{"a.go"}) {
				t.Fatalf("NonTerminal = %v, want [a.go]", r.NonTerminal)
			}
			if len(r.Missing) != 0 {
				t.Fatalf("Missing = %v, want empty (present-but-incomplete is not missing)", r.Missing)
			}
		})
	}
}

// Ambiguous declared identity (two items normalize to the same ID) is a defect:
// the comparison cannot be trusted, so Complete is false.
func TestValidate_AmbiguousDeclaredIdentity(t *testing.T) {
	declared := []DeclaredScopeItem{
		item("a.go", ""),
		item("./a.go", ""), // path.Clean reduces "./a.go" to "a.go" -> collision
	}
	disp := []CoverageDisposition{dstat("a.go", StatusExamined)}
	r := Validate(declared, disp, false)

	if r.Complete {
		t.Fatalf("ambiguous declared identity must not be complete")
	}
	if !reflect.DeepEqual(r.AmbiguousDeclared, []string{"a.go"}) {
		t.Fatalf("AmbiguousDeclared = %v, want [a.go]", r.AmbiguousDeclared)
	}
}

// Duplicate dispositions for one ID are flagged; resolved status is worst-case.
func TestValidate_DuplicateDispositions(t *testing.T) {
	declared := []DeclaredScopeItem{item("a.go", "")}
	disp := []CoverageDisposition{
		dstat("a.go", StatusExamined),
		dstat("a.go", StatusNotExamined), // duplicate -> worst-case non-terminal
	}
	r := Validate(declared, disp, false)

	if r.Complete {
		t.Fatalf("duplicate + non-terminal must not be complete")
	}
	if !reflect.DeepEqual(r.DuplicateDispositions, []string{"a.go"}) {
		t.Fatalf("DuplicateDispositions = %v, want [a.go]", r.DuplicateDispositions)
	}
	if !reflect.DeepEqual(r.NonTerminal, []string{"a.go"}) {
		t.Fatalf("NonTerminal = %v, want [a.go] (worst-case resolution)", r.NonTerminal)
	}
}

// Concern-qualified identity: path#concern is a distinct item from the whole
// file. A whole-file disposition does not cover a concern-qualified item.
func TestValidate_ConcernQualifiedIdentity(t *testing.T) {
	declared := []DeclaredScopeItem{
		item("a.go", ""),        // whole file
		item("a.go", "exports"), // specific concern
	}
	dispFull := []CoverageDisposition{
		dstat("a.go", StatusExamined),         // covers whole-file only
		dstat("a.go#exports", StatusExamined), // covers the concern
	}
	r := Validate(declared, dispFull, false)
	if !r.Complete {
		t.Fatalf("concern-qualified items both covered should be complete; got %+v", r)
	}

	// Dropping the concern disposition leaves the concern item missing.
	dispPartial := []CoverageDisposition{dstat("a.go", StatusExamined)}
	r2 := Validate(declared, dispPartial, false)
	if r2.Complete {
		t.Fatalf("missing concern disposition must not be complete")
	}
	if !reflect.DeepEqual(r2.Missing, []string{"a.go#exports"}) {
		t.Fatalf("Missing = %v, want [a.go#exports]", r2.Missing)
	}
}

// Fixture 5: the validator claims structural coverage only, not semantic
// examination. The CoverageReport type carries NO field that could be read as
// a semantic/quality verdict — only structural gap classes. Complete means
// "every declared item was accounted for," never "meaningfully examined." This
// test PINS that contract with a reflect.TypeOf field-set assertion: adding a
// quality/semantic field (Quality, Semantic, Attention, Confidence, Depth, ...)
// becomes a RED test that must be updated INTENTIONALLY, not silently.
func TestValidate_Fixture5_StructuralCoverageOnlyNotSemantic(t *testing.T) {
	// The CoverageReport field set is exactly the structural gap classes plus
	// Complete and the informational FailFastTerminated flag. If a field is
	// added or removed, this assertion fails and forces a deliberate review of
	// the validator's structural-only authority.
	wantFields := []string{
		"Complete",
		"Missing",
		"NonTerminal",
		"Extra",
		"AmbiguousDeclared",
		"DuplicateDispositions",
		"UnexplainedExclusions",
		"FailFastTerminated",
	}
	got := []string{}
	tt := reflect.TypeOf(CoverageReport{})
	for i := 0; i < tt.NumField(); i++ {
		got = append(got, tt.Field(i).Name)
	}
	if !reflect.DeepEqual(got, wantFields) {
		t.Fatalf("CoverageReport field set drifted (structural-only authority):\n got  %v\n want %v", got, wantFields)
	}

	// A complete report where an item was merely "examined" (structurally
	// accounted for) carries no claim about HOW it was examined.
	declared := []DeclaredScopeItem{item("a.go", "")}
	dp := []CoverageDisposition{dstat("a.go", StatusExamined)}
	r := Validate(declared, dp, false)
	if !r.Complete {
		t.Fatalf("expected complete")
	}
}

// Determinism: identical inputs yield identical outputs across calls, and
// disposition input order does not affect the result.
func TestValidate_DeterministicAndOrderIndependent(t *testing.T) {
	declared := []DeclaredScopeItem{
		item("a.go", ""),
		item("b.go", ""),
		item("c.go", ""),
	}
	dispOrder1 := []CoverageDisposition{
		dstat("a.go", StatusExamined),
		dexcl("b.go", "out of slice"),
		dstat("c.go", StatusExamined),
	}
	dispOrder2 := []CoverageDisposition{
		dstat("c.go", StatusExamined),
		dstat("a.go", StatusExamined),
		dexcl("b.go", "out of slice"),
	}
	r1 := Validate(declared, dispOrder1, false)
	r2 := Validate(declared, dispOrder2, false)
	if !reflect.DeepEqual(r1, r2) {
		t.Fatalf("order-independent: r1=%+v r2=%+v", r1, r2)
	}
	if !r1.Complete {
		t.Fatalf("expected complete")
	}
}

// Unknown disposition status is treated as non-terminal (fail-safe): a garbage
// status cannot smuggle an item to terminal/complete.
func TestValidate_UnknownStatusIsNonTerminal(t *testing.T) {
	declared := []DeclaredScopeItem{item("a.go", "")}
	dp := []CoverageDisposition{{ItemID: "a.go", Status: DispositionStatus("bogus")}}
	r := Validate(declared, dp, false)
	if r.Complete {
		t.Fatalf("unknown status must be treated as non-terminal (fail-safe)")
	}
	if !reflect.DeepEqual(r.NonTerminal, []string{"a.go"}) {
		t.Fatalf("NonTerminal = %v, want [a.go]", r.NonTerminal)
	}
}

func TestIsTerminal(t *testing.T) {
	cases := []struct {
		s    DispositionStatus
		want bool
	}{
		{StatusExamined, true},
		{StatusExcluded, true},
		{StatusNotExamined, false},
		{StatusBlocked, false},
		{DispositionStatus("nope"), false},
		{DispositionStatus(""), false},
	}
	for _, tc := range cases {
		if got := tc.s.IsTerminal(); got != tc.want {
			t.Errorf("IsTerminal(%q) = %v, want %v", tc.s, got, tc.want)
		}
	}
}

func TestDeclaredScopeItemID(t *testing.T) {
	cases := []struct {
		name string
		item DeclaredScopeItem
		want string
	}{
		{"whole file", DeclaredScopeItem{Path: "a/b.go"}, "a/b.go"},
		{"with concern", DeclaredScopeItem{Path: "a/b.go", Concern: "exports"}, "a/b.go#exports"},
		{"concern trimmed", DeclaredScopeItem{Path: "a/b.go", Concern: "  exports  "}, "a/b.go#exports"},
		{"backslashes normalized", DeclaredScopeItem{Path: "a\\b.go"}, "a/b.go"},
		{"dotsegment cleaned", DeclaredScopeItem{Path: "a/./b.go"}, "a/b.go"},
		{"trailing slash cleaned", DeclaredScopeItem{Path: "a/b.go/", Concern: ""}, "a/b.go"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.item.ID(); got != tc.want {
				t.Errorf("ID() = %q, want %q", got, tc.want)
			}
		})
	}
}

// HasGaps is the single predicate a future approval gate would consult, and is
// the clean negation of Complete (minus the informational fail-fast flag).
func TestHasGaps(t *testing.T) {
	if (CoverageReport{}).HasGaps() {
		t.Fatal("zero-value report has no gaps")
	}
	if !(CoverageReport{Missing: []string{"x"}}).HasGaps() {
		t.Fatal("report with missing must have gaps")
	}
	if !(CoverageReport{UnexplainedExclusions: []string{"x"}}).HasGaps() {
		t.Fatal("report with unexplained exclusion must have gaps")
	}
	// FailFastTerminated alone is informational, not a gap.
	if (CoverageReport{FailFastTerminated: true}).HasGaps() {
		t.Fatal("fail-fast flag alone is not a gap")
	}
}

// TestValidate_Integration_RealisticReviewSlice exercises Validate end-to-end
// over a realistic commit-review declared scope — the shape a tiered
// commit-reviewer would actually produce — rather than the minimal 2-3-item
// abstract fixtures above. It proves the F4-A declared-scope-coverage property
// holds over a realistic input even though the validator is not yet wired to
// any blocking approval gate (it remains INFORMS-only / non-blocking per the
// decision memo's gate-attachment preconditions).
//
// The realistic slice mixes the two terminal dispositions a real review uses:
//
//   - StatusExamined  for files the reviewer actually read
//   - StatusExcluded  (WITH a non-blank Reason) for files excluded by contract
//     (generated/vendored artifact, out-of-slice docs)
//
// plus a concern-qualified item (a sub-region of a file the review promised to
// cover specifically). The sub-tests then assert:
//
//  1. COMPLETE realistic coverage — every declared item accounted for ->
//     Complete=true and every gap slice empty (the happy path a future approval
//     gate would consult via HasGaps).
//  2. REALISTIC GAP, a missed item — dropping one examined disposition (a
//     reviewer who forgot to account for one declared file) is caught: Missing
//     grows to name it and Complete=false. This is the load-bearing coverage
//     property: a missed item cannot hide behind the other covered items.
//  3. REALISTIC excluded-by-contract — a generated file excluded WITH a reason
//     is terminal and not a gap; the SAME exclusion WITHOUT a reason becomes an
//     unexplained exclusion that blocks Complete (the F1 fix, exercised over a
//     realistic reason shape: "generated file" must carry its contract).
//
// This complements the minimal unit fixtures above: those prove each gap class
// in isolation with abstract inputs; this proves the classes compose correctly
// and that happy-path coverage + gap detection hold over a realistic review
// slice. It does NOT fabricate coverage semantics — every assertion exercises
// real declared-scope-coverage behavior of Validate.
func TestValidate_Integration_RealisticReviewSlice(t *testing.T) {
	// A realistic commit-review declared scope: hand-written source, a test
	// file, the validator package (whole + a concern-qualified sub-region), a
	// sibling package, an out-of-slice doc, and a generated artifact.
	declared := []DeclaredScopeItem{
		item("internal/cli/release.go", ""),
		item("internal/cli/release_test.go", ""),
		item("internal/scopecoverage/scopecoverage.go", ""),
		item("internal/scopecoverage/scopecoverage.go", "exports"), // concern-qualified
		item("internal/ownership/classify.go", ""),
		item("docs/ai/release-flow.md", ""),
		item("internal/cli/release_string.go", ""), // generated
	}

	// Canonical IDs the dispositions target (path.Cleaned; concern joined by #).
	const (
		releaseGo           = "internal/cli/release.go"
		releaseTestGo       = "internal/cli/release_test.go"
		scopecoverageGo     = "internal/scopecoverage/scopecoverage.go"
		scopecoverageExport = "internal/scopecoverage/scopecoverage.go#exports"
		classifyGo          = "internal/ownership/classify.go"
		releaseFlowMd       = "docs/ai/release-flow.md"
		releaseStringGo     = "internal/cli/release_string.go"
	)

	// Dispositions a real review would produce: examined for hand-written code,
	// excluded-by-contract (WITH reason) for the generated artifact and the
	// out-of-slice doc, examined for the concern-qualified sub-region.
	completeDispositions := []CoverageDisposition{
		dstat(releaseGo, StatusExamined),
		dstat(releaseTestGo, StatusExamined),
		dstat(scopecoverageGo, StatusExamined),
		dstat(scopecoverageExport, StatusExamined),
		dstat(classifyGo, StatusExamined),
		dexcl(releaseFlowMd, "out of slice: docs-only change, no code surface reviewed"),
		dexcl(releaseStringGo, "generated file: regenerated from source, not hand-reviewed"),
	}

	// (1) COMPLETE realistic coverage: every declared item accounted for.
	t.Run("complete_realistic_coverage", func(t *testing.T) {
		r := Validate(declared, completeDispositions, false)
		if !r.Complete {
			t.Fatalf("realistic full coverage must be Complete; got %+v", r)
		}
		if r.HasGaps() {
			t.Fatalf("HasGaps must be false for complete realistic coverage; got %+v", r)
		}
		for _, gap := range []struct {
			name string
			got  []string
		}{
			{"Missing", r.Missing},
			{"NonTerminal", r.NonTerminal},
			{"Extra", r.Extra},
			{"AmbiguousDeclared", r.AmbiguousDeclared},
			{"DuplicateDispositions", r.DuplicateDispositions},
			{"UnexplainedExclusions", r.UnexplainedExclusions},
		} {
			if len(gap.got) != 0 {
				t.Errorf("%s = %v, want empty for complete realistic coverage", gap.name, gap.got)
			}
		}
	})

	// (2) REALISTIC GAP — a missed item: the reviewer forgot to account for
	// internal/ownership/classify.go (no disposition). The validator must catch
	// it: Complete=false, Missing names the missed file, and no other item's
	// coverage hides the gap.
	t.Run("realistic_gap_missed_item_caught", func(t *testing.T) {
		missed := classifyGo
		disp := dropDisposition(completeDispositions, missed)
		r := Validate(declared, disp, false)
		if r.Complete {
			t.Fatalf("a missed declared item must block Complete; got %+v", r)
		}
		if !reflect.DeepEqual(r.Missing, []string{missed}) {
			t.Fatalf("Missing = %v, want [%s]", r.Missing, missed)
		}
		// The other covered items must not contaminate the gap slices.
		if len(r.Extra) != 0 || len(r.NonTerminal) != 0 || len(r.UnexplainedExclusions) != 0 {
			t.Fatalf("only Missing should flag a missed item; got Extra=%v NonTerminal=%v UnexplainedExclusions=%v",
				r.Extra, r.NonTerminal, r.UnexplainedExclusions)
		}
	})

	// (3) REALISTIC excluded-by-contract: the generated file excluded WITH a
	// reason is terminal (no gap). The SAME exclusion WITHOUT a reason becomes
	// an unexplained exclusion that blocks Complete — over the realistic reason
	// shape, "generated file" must carry its contract.
	t.Run("realistic_excluded_by_contract_reason_is_load_bearing", func(t *testing.T) {
		// With reason -> terminal, full coverage still complete.
		rWithReason := Validate(declared, completeDispositions, false)
		if !rWithReason.Complete {
			t.Fatalf("generated file excluded WITH reason must be terminal (complete); got %+v", rWithReason)
		}
		if len(rWithReason.UnexplainedExclusions) != 0 {
			t.Fatalf("no unexplained exclusions expected when reason present; got %v", rWithReason.UnexplainedExclusions)
		}

		// Without reason -> unexplained exclusion blocks Complete.
		dispNoReason := replaceDisposition(completeDispositions, releaseStringGo,
			CoverageDisposition{ItemID: releaseStringGo, Status: StatusExcluded, Reason: ""})
		rNoReason := Validate(declared, dispNoReason, false)
		if rNoReason.Complete {
			t.Fatalf("generated file excluded WITHOUT reason must block Complete; got %+v", rNoReason)
		}
		if !reflect.DeepEqual(rNoReason.UnexplainedExclusions, []string{releaseStringGo}) {
			t.Fatalf("UnexplainedExclusions = %v, want [%s]", rNoReason.UnexplainedExclusions, releaseStringGo)
		}
		if !reflect.DeepEqual(rNoReason.NonTerminal, []string{releaseStringGo}) {
			t.Fatalf("NonTerminal = %v, want [%s]", rNoReason.NonTerminal, releaseStringGo)
		}
		// Everything else is still covered: only the unexplained exclusion flags.
		if len(rNoReason.Missing) != 0 || len(rNoReason.Extra) != 0 {
			t.Fatalf("only the unexplained exclusion should flag; got Missing=%v Extra=%v",
				rNoReason.Missing, rNoReason.Extra)
		}
	})
}

// dropDisposition returns a copy of disp with the single entry whose ItemID
// equals id removed. It models a realistic missed item (a reviewer who forgot
// one declared file). It panics if id is absent so a fixture typo fails loudly
// rather than passing vacuously.
func dropDisposition(disp []CoverageDisposition, id string) []CoverageDisposition {
	out := make([]CoverageDisposition, 0, len(disp))
	removed := false
	for _, d := range disp {
		if d.ItemID == id {
			removed = true
			continue
		}
		out = append(out, d)
	}
	if !removed {
		panic("dropDisposition: id " + id + " not present (fixture typo)")
	}
	return out
}

// replaceDisposition returns a copy of disp with the single entry whose ItemID
// equals id replaced by repl. It models mutating one realistic disposition
// (e.g. stripping an exclusion reason) while keeping the rest. It panics if id
// is absent so a fixture typo fails loudly rather than passing vacuously.
func replaceDisposition(disp []CoverageDisposition, id string, repl CoverageDisposition) []CoverageDisposition {
	out := make([]CoverageDisposition, 0, len(disp))
	replaced := false
	for _, d := range disp {
		if d.ItemID == id {
			out = append(out, repl)
			replaced = true
			continue
		}
		out = append(out, d)
	}
	if !replaced {
		panic("replaceDisposition: id " + id + " not present (fixture typo)")
	}
	return out
}
