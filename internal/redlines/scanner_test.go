package redlines

import (
	"bytes"
	"reflect"
	"strings"
	"testing"
)

// All identifiers and terms in this file are OBVIOUSLY synthetic (synthetic-*,
// subj-test-*). No real registry entry is represented.

// syntheticTermsLeakCheck is the grep-test invariant: every Finding returned by
// Scan must contain ONLY an opaque id + reason code + path. NONE of the real
// (synthetic, standing in for private) terms used in fixtures may appear in the
// engine-PRODUCED fields (SubjectID, Reason). The Path field is the file's
// actual path — explicitly "safe metadata" in the Finding contract — so it is
// excluded from the term check (a path-fragment label that matches a path is
// supposed to appear in the Path; that is the file's real location, not an
// engine-injected term). This is the load-bearing security assertion of the
// engine: the engine never ECHOES the matched term.
func assertFindingsLeakNoTerms(t *testing.T, findings []Finding, bannedTerms []string) {
	t.Helper()
	for _, f := range findings {
		for _, term := range bannedTerms {
			lowerTerm := strings.ToLower(term)
			// SubjectID and Reason are engine-produced and must be term-free.
			if strings.Contains(strings.ToLower(f.SubjectID), lowerTerm) ||
				strings.Contains(strings.ToLower(f.Reason), lowerTerm) {
				t.Errorf("LEAK: finding %+v contains banned term %q in SubjectID/Reason", f, term)
			}
			// Path: a CONTENT term (no "/") must never appear here — the engine
			// must not inject the matched content term into the path. A
			// path-fragment term (contains "/") is exempt: it is the file's real
			// path when path-fragment matching fires, which is expected.
			if !strings.Contains(term, "/") &&
				strings.Contains(strings.ToLower(f.Path), lowerTerm) {
				t.Errorf("LEAK: finding Path %q contains content-term %q (engine must not inject the matched term into the path)", f.Path, term)
			}
		}
		if f.Reason != ReasonScrubTerm && f.Reason != ReasonRelationCoOccurrence && f.Reason != ReasonRelationAmbientSideB {
			t.Errorf("finding has unexpected reason %q: %+v", f.Reason, f)
		}
	}
}

// scrubSubject builds an in-memory scrub-project Subject for table tests.
func scrubSubject(id string, labels ...string) Subject {
	return Subject{ID: id, Kind: KindScrubProject, Labels: labels, Policy: policyScrubBeforeCommit}
}

// relationSubject builds an in-memory forbidden-relation Subject.
func relationSubject(id string, sideA, sideB []string) Subject {
	return Subject{ID: id, Kind: KindForbiddenRelation, SideA: sideA, SideB: sideB}
}

func TestScan_ScrubExactTermHit(t *testing.T) {
	s := scrubSubject("subj-test-scrub", "synthetic-alpha", "synthetic-beta")
	ctx := ScanContext{}
	units := []ScanUnit{
		{Path: "src/file.go", Content: []byte("contains the term synthetic-alpha here")},
	}
	got := Scan(ctx, []Subject{s}, units)
	if len(got) != 1 {
		t.Fatalf("want 1 finding, got %d: %+v", len(got), got)
	}
	want := Finding{SubjectID: "subj-test-scrub", Reason: ReasonScrubTerm, Path: "src/file.go"}
	if got[0] != want {
		t.Errorf("got %+v want %+v", got[0], want)
	}
	assertFindingsLeakNoTerms(t, got, []string{"synthetic-alpha", "synthetic-beta"})
}

func TestScan_ScrubAliasHit(t *testing.T) {
	// The Labels slice is the whole term set; the registry supplies aliases as
	// additional labels. Any configured label hit = violation.
	s := scrubSubject("subj-test-scrub", "synthetic-primary", "synthetic-alias")
	ctx := ScanContext{}
	units := []ScanUnit{
		{Path: "doc.md", Content: []byte("mentions synthetic-alias inline")},
	}
	got := Scan(ctx, []Subject{s}, units)
	if len(got) != 1 {
		t.Fatalf("want 1 finding for alias hit, got %d: %+v", len(got), got)
	}
	assertFindingsLeakNoTerms(t, got, []string{"synthetic-primary", "synthetic-alias"})
}

func TestScan_ScrubCaseInsensitive(t *testing.T) {
	s := scrubSubject("subj-test-scrub", "Synthetic-Alpha")
	ctx := ScanContext{}
	cases := []string{
		"synthetic-alpha lower",
		"SYNTHETIC-ALPHA upper",
		"prefix Synthetic-Alpha mixed",
		"SYNTHETIC-alpha partial",
	}
	for _, c := range cases {
		c := c
		t.Run(c, func(t *testing.T) {
			got := Scan(ctx, []Subject{s}, []ScanUnit{{Path: "f", Content: []byte(c)}})
			if len(got) != 1 {
				t.Fatalf("case-insensitive match expected for %q, got %d findings", c, len(got))
			}
		})
	}
	assertFindingsLeakNoTerms(t, nil, []string{"Synthetic-Alpha"})
}

func TestScan_ScrubPathFragmentHit(t *testing.T) {
	// A label containing "/" is a path fragment; it matches the unit path even
	// when the content is clean.
	s := scrubSubject("subj-test-scrub", "sensitiveproj/src")
	ctx := ScanContext{}
	units := []ScanUnit{
		{Path: "sensitiveproj/src/main.go", Content: []byte("clean content")},
	}
	got := Scan(ctx, []Subject{s}, units)
	if len(got) != 1 {
		t.Fatalf("path-fragment label should match the path, got %d: %+v", len(got), got)
	}
	assertFindingsLeakNoTerms(t, got, []string{"sensitiveproj/src"})
}

func TestScan_ScrubPathFragmentCaseInsensitive(t *testing.T) {
	s := scrubSubject("subj-test-scrub", "SensitiveProj/Src")
	ctx := ScanContext{}
	units := []ScanUnit{
		{Path: "sensitiveproj/src/x", Content: []byte("clean")},
	}
	got := Scan(ctx, []Subject{s}, units)
	if len(got) != 1 {
		t.Fatalf("case-insensitive path-fragment match expected, got %d: %+v", len(got), got)
	}
}

func TestScan_ScrubNoHitWhenClean(t *testing.T) {
	s := scrubSubject("subj-test-scrub", "synthetic-alpha")
	ctx := ScanContext{}
	units := []ScanUnit{
		{Path: "f.go", Content: []byte("totally unrelated content")},
	}
	got := Scan(ctx, []Subject{s}, units)
	if len(got) != 0 {
		t.Fatalf("clean content should not match, got %+v", got)
	}
}

func TestScan_RelationSameUnitHit(t *testing.T) {
	// SideA AND SideB co-occurring within the SAME unit -> violation.
	s := relationSubject("subj-test-rel", []string{"synthetic-org-alpha"}, []string{"synthetic-domain-beta"})
	ctx := ScanContext{}
	units := []ScanUnit{
		{Path: "README.md", Content: []byte("This mentions synthetic-org-alpha and also synthetic-domain-beta together")},
	}
	got := Scan(ctx, []Subject{s}, units)
	if len(got) != 1 {
		t.Fatalf("same-unit co-occurrence should hit, got %d: %+v", len(got), got)
	}
	want := Finding{SubjectID: "subj-test-rel", Reason: ReasonRelationCoOccurrence, Path: "README.md"}
	if got[0] != want {
		t.Errorf("got %+v want %+v", got[0], want)
	}
	assertFindingsLeakNoTerms(t, got, []string{"synthetic-org-alpha", "synthetic-domain-beta"})
}

func TestScan_RelationSeparateUnitsIsNonHit(t *testing.T) {
	// SideA in one unit, SideB in a different unit -> DOCUMENTED NON-HIT.
	// Cross-unit inference is unsupported in v1 (honesty contract).
	s := relationSubject("subj-test-rel", []string{"synthetic-org-alpha"}, []string{"synthetic-domain-beta"})
	ctx := ScanContext{}
	units := []ScanUnit{
		{Path: "a.md", Content: []byte("only side A: synthetic-org-alpha")},
		{Path: "b.md", Content: []byte("only side B: synthetic-domain-beta")},
	}
	got := Scan(ctx, []Subject{s}, units)
	if len(got) != 0 {
		t.Fatalf("cross-unit co-occurrence is a documented non-hit; got %+v", got)
	}
}

func TestScan_RelationOnlyOneSideIsNonHit(t *testing.T) {
	// Only SideA present (no SideB anywhere) -> no hit (non-ambient).
	s := relationSubject("subj-test-rel", []string{"synthetic-org-alpha"}, []string{"synthetic-domain-beta"})
	ctx := ScanContext{}
	units := []ScanUnit{
		{Path: "a.md", Content: []byte("only side A: synthetic-org-alpha")},
	}
	got := Scan(ctx, []Subject{s}, units)
	if len(got) != 0 {
		t.Fatalf("one-side-only non-ambient should not hit; got %+v", got)
	}
}

func TestScan_RelationAmbientSideBAloneHits(t *testing.T) {
	// When the subject is AMBIENT for the repo (repo identity implies SideA),
	// any SideB term ALONE in any unit -> violation (ReasonRelationAmbientSideB).
	s := relationSubject("subj-test-rel", []string{"synthetic-org-alpha"}, []string{"synthetic-domain-beta"})
	s.AmbientRepos = []string{"github.com/synthetic-org/*"}
	ctx := ScanContext{
		RepoPath: "/home/me/repo",
		Remotes:  []string{"github.com/synthetic-org/myrepo"},
	}
	units := []ScanUnit{
		{Path: "notes.md", Content: []byte("just synthetic-domain-beta on its own")},
	}
	got := Scan(ctx, []Subject{s}, units)
	if len(got) != 1 {
		t.Fatalf("ambient SideB-alone should hit, got %d: %+v", len(got), got)
	}
	want := Finding{SubjectID: "subj-test-rel", Reason: ReasonRelationAmbientSideB, Path: "notes.md"}
	if got[0] != want {
		t.Errorf("got %+v want %+v", got[0], want)
	}
	// SideA must NEVER appear or be echoed in an ambient finding.
	assertFindingsLeakNoTerms(t, got, []string{"synthetic-org-alpha", "synthetic-domain-beta"})
}

func TestScan_RelationAmbientRequiresBindingMatch(t *testing.T) {
	// A repo that does NOT match ambient_repos is NOT ambient: SideA must still
	// co-occur. This proves ambient degeneration is derived from explicit
	// binding, never guessed.
	s := relationSubject("subj-test-rel", []string{"synthetic-org-alpha"}, []string{"synthetic-domain-beta"})
	s.AmbientRepos = []string{"github.com/synthetic-org/*"}
	ctx := ScanContext{
		Remotes: []string{"github.com/different-org/repo"},
	}
	units := []ScanUnit{
		{Path: "x.md", Content: []byte("just synthetic-domain-beta")},
	}
	got := Scan(ctx, []Subject{s}, units)
	if len(got) != 0 {
		t.Fatalf("non-matching repo should NOT be ambient; SideB-alone must not hit; got %+v", got)
	}
}

func TestScan_DeterministicOrdering(t *testing.T) {
	// Findings sort by Path then SubjectID, deterministically, regardless of
	// input order or how many labels matched.
	s1 := scrubSubject("subj-zeta", "synthetic-alpha")
	s2 := scrubSubject("subj-alpha", "synthetic-alpha")
	ctx := ScanContext{}
	units := []ScanUnit{
		{Path: "z/file.go", Content: []byte("synthetic-alpha")},
		{Path: "a/file.go", Content: []byte("synthetic-alpha")},
		{Path: "m/file.go", Content: []byte("synthetic-alpha")},
	}
	got := Scan(ctx, []Subject{s1, s2}, units)
	// 3 paths x 2 subjects = 6 findings, sorted by Path then SubjectID.
	if len(got) != 6 {
		t.Fatalf("want 6 findings, got %d: %+v", len(got), got)
	}
	wantOrder := []struct{ path, subj string }{
		{"a/file.go", "subj-alpha"},
		{"a/file.go", "subj-zeta"},
		{"m/file.go", "subj-alpha"},
		{"m/file.go", "subj-zeta"},
		{"z/file.go", "subj-alpha"},
		{"z/file.go", "subj-zeta"},
	}
	for i, w := range wantOrder {
		if got[i].Path != w.path || got[i].SubjectID != w.subj {
			t.Errorf("finding[%d] = {%s %s}, want {%s %s}", i, got[i].Path, got[i].SubjectID, w.path, w.subj)
		}
	}
}

func TestScan_DedupCollapsesMultiLabelHits(t *testing.T) {
	// Two labels of the SAME subject both hit in one unit -> ONE finding (the
	// Finding carries no term, so duplicates would be byte-identical noise).
	s := scrubSubject("subj-test-scrub", "synthetic-alpha", "synthetic-beta")
	ctx := ScanContext{}
	units := []ScanUnit{
		{Path: "f.go", Content: []byte("synthetic-alpha and synthetic-beta both present")},
	}
	got := Scan(ctx, []Subject{s}, units)
	if len(got) != 1 {
		t.Fatalf("multi-label same-subject same-unit must dedup to 1, got %d: %+v", len(got), got)
	}
	assertFindingsLeakNoTerms(t, got, []string{"synthetic-alpha", "synthetic-beta"})
}

func TestScan_BinaryUnitSkipped(t *testing.T) {
	// A unit whose content contains a NUL byte is binary and is skipped: no
	// finding for any subject, even if a term appears around the NUL.
	s := scrubSubject("subj-test-scrub", "synthetic-alpha")
	ctx := ScanContext{}
	bin := []byte("synthetic-alpha\x00binary garbage\x00")
	units := []ScanUnit{
		{Path: "blob.bin", Content: bin},
	}
	got := Scan(ctx, []Subject{s}, units)
	if len(got) != 0 {
		t.Fatalf("binary unit must be skipped, got %+v", got)
	}
}

func TestScan_OversizedUnitSkipped(t *testing.T) {
	// A unit above MaxUnitSize is skipped (documented limitation).
	s := scrubSubject("subj-test-scrub", "synthetic-alpha")
	ctx := ScanContext{}
	big := bytes.Repeat([]byte("a"), MaxUnitSize+1)
	big = append(big, []byte("synthetic-alpha")...)
	units := []ScanUnit{
		{Path: "huge.go", Content: big},
	}
	got := Scan(ctx, []Subject{s}, units)
	if len(got) != 0 {
		t.Fatalf("oversized unit must be skipped, got %+v", got)
	}
}

func TestScan_OversizedBoundaryMatchesAtCap(t *testing.T) {
	// A unit exactly at MaxUnitSize is NOT oversized and IS scanned.
	s := scrubSubject("subj-test-scrub", "synthetic-alpha")
	ctx := ScanContext{}
	at := bytes.Repeat([]byte("x"), MaxUnitSize-len("synthetic-alpha"))
	at = append(at, []byte("synthetic-alpha")...)
	if len(at) != MaxUnitSize {
		t.Fatalf("fixture length %d, want %d", len(at), MaxUnitSize)
	}
	got := Scan(ctx, []Subject{s}, []ScanUnit{{Path: "cap.go", Content: at}})
	if len(got) != 1 {
		t.Fatalf("unit at exactly MaxUnitSize should be scanned, got %d findings", len(got))
	}
}

func TestScan_ParaphraseUndetected(t *testing.T) {
	// A paraphrase of a configured term is NOT detected (documented honesty
	// limitation). The engine is lexical-only.
	s := scrubSubject("subj-test-scrub", "synthetic-alpha")
	ctx := ScanContext{}
	// "synthetic alpha" (with a space) is a paraphrase of "synthetic-alpha".
	units := []ScanUnit{
		{Path: "f.md", Content: []byte("a synthetic alpha style reference")},
	}
	got := Scan(ctx, []Subject{s}, units)
	if len(got) != 0 {
		t.Fatalf("paraphrase should be undetected (lexical-only); got %+v", got)
	}
}

func TestScan_TranslationUndetected(t *testing.T) {
	// A translation of a configured term is NOT detected.
	s := scrubSubject("subj-test-scrub", "synthetic-alpha")
	ctx := ScanContext{}
	// Pretend "pseudoalpha" is a foreign-language rendering.
	units := []ScanUnit{
		{Path: "f.md", Content: []byte("a pseudoalpha reference")},
	}
	got := Scan(ctx, []Subject{s}, units)
	if len(got) != 0 {
		t.Fatalf("translation should be undetected (lexical-only); got %+v", got)
	}
}

func TestScan_UndeclaredAliasUndetected(t *testing.T) {
	// An alias the registry did NOT declare is NOT detected. The registry
	// supplies the only terms the engine knows.
	s := scrubSubject("subj-test-scrub", "synthetic-alpha")
	ctx := ScanContext{}
	units := []ScanUnit{
		{Path: "f.md", Content: []byte("uses undeclared-alias-for-the-thing")}, // not in Labels
	}
	got := Scan(ctx, []Subject{s}, units)
	if len(got) != 0 {
		t.Fatalf("undeclared alias should be undetected; got %+v", got)
	}
}

func TestScan_MultipleSubjectsAndKinds(t *testing.T) {
	// A mix of scrub + relation subjects, ambient + non-ambient, across units.
	scrub := scrubSubject("subj-scrub", "synthetic-alpha")
	relNonAmbient := relationSubject("subj-rel", []string{"synthetic-org"}, []string{"synthetic-domain"})
	relAmbient := relationSubject("subj-rel-amb", []string{"synthetic-org"}, []string{"synthetic-domain"})
	relAmbient.AmbientRepos = []string{"/home/me/repo"}

	ctx := ScanContext{RepoPath: "/home/me/repo"}
	units := []ScanUnit{
		{Path: "a.go", Content: []byte("synthetic-alpha present")},
		{Path: "b.go", Content: []byte("synthetic-org and synthetic-domain co-occur")},
		{Path: "c.go", Content: []byte("just synthetic-domain for ambient")},
	}
	got := Scan(ctx, []Subject{scrub, relNonAmbient, relAmbient}, units)
	// Expected:
	//  a.go: subj-scrub scrub-term
	//  b.go: subj-rel relation-co-occurrence; subj-rel-amb relation-co-occurrence (both sides present, ambient also fires co-occurrence? no — see below)
	//  c.go: subj-rel-amb relation-ambient-side-b (ambient, SideB alone)
	//
	// NOTE on b.go for the ambient subject: when SideA AND SideB co-occur AND
	// the subject is ambient, the engine emits the AMBIENT reason (it checks
	// ambient first and short-circuits). This is intentional: ambient is the
	// stronger, more specific case. The non-ambient subject emits co-occurrence.
	wantSet := map[findingKey]bool{
		{"subj-scrub", ReasonScrubTerm, "a.go"}:              true,
		{"subj-rel", ReasonRelationCoOccurrence, "b.go"}:     true,
		{"subj-rel-amb", ReasonRelationAmbientSideB, "b.go"}: true,
		{"subj-rel-amb", ReasonRelationAmbientSideB, "c.go"}: true,
	}
	if len(got) != len(wantSet) {
		t.Fatalf("want %d findings, got %d: %+v", len(wantSet), len(got), got)
	}
	for _, f := range got {
		k := findingKey{f.SubjectID, f.Reason, f.Path}
		if !wantSet[k] {
			t.Errorf("unexpected finding %+v", f)
		}
	}
	assertFindingsLeakNoTerms(t, got, []string{"synthetic-alpha", "synthetic-org", "synthetic-domain"})
}

func TestScan_EmptyInputsProduceEmptyResults(t *testing.T) {
	ctx := ScanContext{}
	if got := Scan(ctx, nil, nil); len(got) != 0 {
		t.Errorf("nil inputs should give no findings, got %+v", got)
	}
	if got := Scan(ctx, []Subject{scrubSubject("subj-x", "synthetic-alpha")}, nil); len(got) != 0 {
		t.Errorf("no units should give no findings, got %+v", got)
	}
	if got := Scan(ctx, nil, []ScanUnit{{Path: "f", Content: []byte("synthetic-alpha")}}); len(got) != 0 {
		t.Errorf("no subjects should give no findings, got %+v", got)
	}
}

func TestScan_EmptyContentScansPathOnly(t *testing.T) {
	// A unit with empty content still has its path checked for path-fragment
	// labels. A content-only label does not match empty content.
	pathLabel := scrubSubject("subj-path", "sensitive/path")
	termLabel := scrubSubject("subj-term", "synthetic-alpha")
	ctx := ScanContext{}
	units := []ScanUnit{
		{Path: "sensitive/path/file", Content: nil},
	}
	got := Scan(ctx, []Subject{pathLabel, termLabel}, units)
	if len(got) != 1 || got[0].SubjectID != "subj-path" {
		t.Fatalf("empty content: only path-fragment label should match, got %+v", got)
	}
	// The path-fragment finding's Path IS the file's path (contains the fragment
	// by design); the content term must NOT appear anywhere.
	assertFindingsLeakNoTerms(t, got, []string{"synthetic-alpha"})
}

// TestScan_NoRealTermLeaksInAnyFinding is the DEDICATED no-leak grep test. It
// produces a comprehensive set of findings (scrub hits, relation hits, ambient
// hits, across many files and subjects) using CONTENT terms that do NOT appear
// in any file path, then greps every Finding field (SubjectID, Reason, AND Path)
// for every real (synthetic) term. NONE may appear. This is the security
// assertion the mission names explicitly.
func TestScan_NoRealTermLeaksInAnyFinding(t *testing.T) {
	// Content terms only — none appear in any unit Path below.
	const scrubTerm1 = "synthetic-scrub-term-one"
	const scrubTerm2 = "synthetic-scrub-term-two"
	const sideA = "synthetic-rel-side-alpha"
	const sideB = "synthetic-rel-side-beta"
	const ambientB = "synthetic-ambient-side-beta"

	scrub := scrubSubject("subj-scrub", scrubTerm1, scrubTerm2)
	rel := relationSubject("subj-rel", []string{sideA}, []string{sideB})
	amb := relationSubject("subj-amb", []string{"synthetic-ambient-side-alpha"}, []string{ambientB})
	amb.AmbientRepos = []string{"github.com/synthetic-org/*"}

	ctx := ScanContext{Remotes: []string{"github.com/synthetic-org/myrepo"}}
	// Paths deliberately contain NONE of the content terms above.
	units := []ScanUnit{
		{Path: "docs/readme.md", Content: []byte(scrubTerm1 + " appears here")},
		{Path: "src/main.go", Content: []byte(scrubTerm2 + " also appears")},
		{Path: "notes/a.txt", Content: []byte(sideA + " and " + sideB + " co-occur")},
		{Path: "config/b.yml", Content: []byte(ambientB + " alone in ambient repo")},
	}
	got := Scan(ctx, []Subject{scrub, rel, amb}, units)
	if len(got) == 0 {
		t.Fatal("expected findings, got none")
	}
	// Every real content term must be absent from EVERY field (SubjectID,
	// Reason, Path). Since none of these terms appear in any unit Path, a hit
	// in the Path field would prove the engine injected the matched term.
	banned := []string{scrubTerm1, scrubTerm2, sideA, sideB, ambientB, "synthetic-ambient-side-alpha"}
	for _, f := range got {
		for _, term := range banned {
			lower := strings.ToLower(term)
			if strings.Contains(strings.ToLower(f.SubjectID), lower) {
				t.Errorf("LEAK SubjectID: %+v contains %q", f, term)
			}
			if strings.Contains(strings.ToLower(f.Reason), lower) {
				t.Errorf("LEAK Reason: %+v contains %q", f, term)
			}
			if strings.Contains(strings.ToLower(f.Path), lower) {
				t.Errorf("LEAK Path: %+v contains %q (engine must not inject the matched term into the path)", f, term)
			}
		}
	}
}

func TestHonestyContractIsNonEmpty(t *testing.T) {
	// Guard: the honesty contract text must be present and contain the key
	// honesty phrases so consumers cannot accidentally ship an empty/stubbed
	// version.
	for _, phrase := range []string{
		"lexical and best-effort",
		"does not detect paraphrases",
		"translations",
		"undeclared aliases",
		"A passing scan is not proof",
	} {
		if !strings.Contains(HonestyContract, phrase) {
			t.Errorf("HonestyContract missing phrase %q", phrase)
		}
	}
}

func TestFindingHasNoTermField(t *testing.T) {
	// Structural guard: the Finding type has EXACTLY three exported fields named
	// SubjectID, Reason, Path. NONE of them may be named in a way that could
	// carry a matched term (no "MatchedTerm", "Term", "Label", "Match", etc.).
	// This catches an accidental field addition that the output-grep test
	// (assertFindingsLeakNoTerms) might not exercise if the new field is left
	// zero-valued in tests.
	allowed := map[string]bool{
		"SubjectID": true,
		"Reason":    true,
		"Path":      true,
	}
	for i := 0; i < reflect.TypeOf(Finding{}).NumField(); i++ {
		name := reflect.TypeOf(Finding{}).Field(i).Name
		if !allowed[name] {
			t.Errorf("Finding has field %q not in the allowed set {SubjectID, Reason, Path} — a term-carrying field may have been added", name)
		}
	}
	if n := reflect.TypeOf(Finding{}).NumField(); n != 3 {
		t.Errorf("Finding must have exactly 3 fields, got %d", n)
	}
}
