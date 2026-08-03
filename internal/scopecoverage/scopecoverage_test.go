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
