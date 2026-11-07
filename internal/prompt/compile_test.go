package prompt

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func mustAssemble(t *testing.T, a *Assembler, vars map[string]string) []Section {
	t.Helper()
	sections, err := a.assembleForTest(vars)
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	return sections
}

func baseAssembler(t *testing.T) *Assembler {
	t.Helper()
	a := NewAssembler()
	for _, s := range []Section{
		{Number: -100, Key: "identity", Owner: "core", Body: "You are the harness kernel.", CacheStable: true},
		{Number: 0, Key: "persona", Owner: "core", Body: "Operate predictably.", Required: true, CacheStable: true},
		{Number: 100, Key: "tools.guidance", Owner: "core", Body: "Use read for files and write for edits."},
	} {
		if err := a.Register(s); err != nil {
			t.Fatalf("Register(%q): %v", s.Key, err)
		}
	}
	return a
}

func baseCatalog(t *testing.T) ToolCatalog {
	t.Helper()
	cat, err := NewToolCatalog([]ToolSummary{
		{Name: "read", Description: "read files"},
		{Name: "write", Description: "write files"},
	})
	if err != nil {
		t.Fatalf("NewToolCatalog: %v", err)
	}
	return cat
}

func identityVersioned() VersionedOptimizer {
	return VersionedOptimizer{Version: "identity-fake/1", Fn: identityFake}
}

func TestCompileHappyPathWritesArtifact(t *testing.T) {
	dir := t.TempDir()
	a, cat := baseAssembler(t), baseCatalog(t)
	ctx := context.Background()

	art, err := Compile(ctx, a, nil, cat, identityVersioned(), InvariantsContract{}, dir)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if art.Hash == "" || art.OptimizerVersion != "identity-fake/1" {
		t.Fatalf("artifact header wrong: %+v", art)
	}
	if art.Tokens.InputTokens <= 0 || art.Tokens.OutputTokens <= 0 {
		t.Fatalf("token accounting missing: %+v", art.Tokens)
	}
	if art.Tokens.DeltaTokens != art.Tokens.InputTokens-art.Tokens.OutputTokens {
		t.Fatalf("delta must equal input-output: %+v", art.Tokens)
	}
	if len(art.Sections) != 3 {
		t.Fatalf("per-section table must cover all sections; got %d", len(art.Sections))
	}
	for _, iv := range art.Invariants {
		if !iv.Passed {
			t.Fatalf("invariant %s must pass on happy path: %s", iv.Name, iv.Detail)
		}
	}

	// Artifact file exists on disk under the content hash.
	if _, err := os.Stat(filepath.Join(dir, "prompt-"+art.Hash+".json")); err != nil {
		t.Fatalf("artifact file missing: %v", err)
	}
}

func TestInputHashInvalidatesOnEveryInputChange(t *testing.T) {
	a, cat := baseAssembler(t), baseCatalog(t)
	base, err := InputHash(a, nil, cat, "v1", InvariantsContract{})
	if err != nil {
		t.Fatalf("InputHash: %v", err)
	}

	// Section body change.
	a2 := baseAssembler(t)
	_ = a2.Register(Section{Number: 500, Key: "extra", Body: "more"})
	if h, _ := InputHash(a2, nil, cat, "v1", InvariantsContract{}); h == base {
		t.Fatal("section-set change must change the hash")
	}
	// Tool catalog change.
	cat2, _ := NewToolCatalog(append(baseCatalog(t).Tools, ToolSummary{Name: "glob", Description: "x"}))
	if h, _ := InputHash(a, nil, cat2, "v1", InvariantsContract{}); h == base {
		t.Fatal("catalog change must change the hash")
	}
	// Optimizer version change.
	if h, _ := InputHash(a, nil, cat, "v2", InvariantsContract{}); h == base {
		t.Fatal("optimizer version change must change the hash")
	}
	// Invariants contract change.
	if h, _ := InputHash(a, nil, cat, "v1", InvariantsContract{DelegatedTools: []string{"write"}}); h == base {
		t.Fatal("contract change must change the hash")
	}
	// Interpolation variable change: the hash is content-addressed — it
	// changes exactly when the interpolated content changes.
	a3 := baseAssembler(t)
	if err := a3.Register(Section{Number: 200, Key: "variant", Body: "Build flavor: {{variant}}."}); err != nil {
		t.Fatal(err)
	}
	if h, _ := InputHash(a3, map[string]string{"variant": "one"}, cat, "v1", InvariantsContract{}); h == base {
		t.Fatal("vars change must change the hash when bodies embed them")
	}
	hOne, _ := InputHash(a3, map[string]string{"variant": "one"}, cat, "v1", InvariantsContract{})
	hTwo, _ := InputHash(a3, map[string]string{"variant": "two"}, cat, "v1", InvariantsContract{})
	if hOne == hTwo {
		t.Fatal("different interpolations of the same section must hash differently")
	}
	hExtra, _ := InputHash(a3, map[string]string{"variant": "one", "unused": "x"}, cat, "v1", InvariantsContract{})
	if hExtra != hOne {
		t.Fatal("unused vars must not change the hash (content addressing)")
	}
}

// failClosed runs Compile with a hostile fake and asserts an InvariantError
// naming the invariant, plus that NO artifact was written.
func failClosed(t *testing.T, name string, fn Optimizer) {
	t.Helper()
	dir := t.TempDir()
	a, cat := baseAssembler(t), baseCatalog(t)
	_, err := Compile(context.Background(), a, nil, cat, VersionedOptimizer{Version: "hostile/1", Fn: fn}, InvariantsContract{}, dir)
	if err == nil {
		t.Fatalf("%s: compile must fail closed", name)
	}
	var inv *InvariantError
	if !errors.As(err, &inv) {
		t.Fatalf("%s: error must be *InvariantError; got %T: %v", name, err, err)
	}
	found := false
	for _, v := range inv.Violations {
		if strings.Contains(v, name) {
			found = true
		}
	}
	if !found {
		t.Fatalf("%s: violations must name the invariant; got %v", name, inv.Violations)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Fatalf("%s: no artifact may be written on violation; dir has %d entries", name, len(entries))
	}
}

func TestInvariantMissingToolReferenceFailsClosed(t *testing.T) {
	failClosed(t, "tools-referenced-or-delegated", func(_ context.Context, in OptimizeInput) (OptimizedPrompt, error) {
		res, _ := identityFake(context.Background(), in)
		res.Bytes = []byte("All tool guidance was removed.") // no read, no write
		res.ReferencedTools = nil
		return res, nil
	})
}

func TestInvariantSubstringsDoNotSatisfyToolReference(t *testing.T) {
	failClosed(t, "tools-referenced-or-delegated", func(_ context.Context, in OptimizeInput) (OptimizedPrompt, error) {
		res, _ := identityFake(context.Background(), in)
		// "already" contains the substring "read" but not as a word.
		res.Bytes = []byte("You already browsed everything; writ errors happen.")
		res.ReferencedTools = nil
		return res, nil
	})
}

func TestInvariantDelegatedToolAbsenceAllowed(t *testing.T) {
	dir := t.TempDir()
	a, cat := baseAssembler(t), baseCatalog(t)
	fn := func(_ context.Context, in OptimizeInput) (OptimizedPrompt, error) {
		res, _ := identityFake(context.Background(), in)
		res.Bytes = []byte("Only read guidance survives.")
		res.ReferencedTools = []string{"read"}
		return res, nil
	}
	contract := InvariantsContract{DelegatedTools: []string{"write"}}
	if _, err := Compile(context.Background(), a, nil, cat, VersionedOptimizer{Version: "delegate/1", Fn: fn}, contract, dir); err != nil {
		t.Fatalf("delegated tool absence must be allowed: %v", err)
	}
}

func TestInvariantInventedToolNameFailsClosed(t *testing.T) {
	failClosed(t, "no-invented-tool-references", func(_ context.Context, in OptimizeInput) (OptimizedPrompt, error) {
		res, _ := identityFake(context.Background(), in)
		res.ReferencedTools = append(res.ReferencedTools, "not_a_real_tool")
		return res, nil
	})
}

func TestInvariantRequiredSectionNeedsRationale(t *testing.T) {
	failClosed(t, "required-sections-preserved-or-rationaled", func(_ context.Context, in OptimizeInput) (OptimizedPrompt, error) {
		res, _ := identityFake(context.Background(), in)
		for i, o := range res.SectionOutcomes {
			if o.Key == "persona" { // Required
				res.SectionOutcomes[i].Action = ActionDropped
				res.SectionOutcomes[i].Rationale = ""
			}
		}
		return res, nil
	})
}

func TestInvariantRequiredSectionDroppedWithRationalePasses(t *testing.T) {
	dir := t.TempDir()
	a, cat := baseAssembler(t), baseCatalog(t)
	fn := func(_ context.Context, in OptimizeInput) (OptimizedPrompt, error) {
		res, _ := identityFake(context.Background(), in)
		for i, o := range res.SectionOutcomes {
			if o.Key == "persona" {
				res.SectionOutcomes[i].Action = ActionDropped
				res.SectionOutcomes[i].Rationale = "persona folded into identity section verbatim"
			}
		}
		return res, nil
	}
	art, err := Compile(context.Background(), a, nil, cat, VersionedOptimizer{Version: "rationale/1", Fn: fn}, InvariantsContract{}, dir)
	if err != nil {
		t.Fatalf("Required drop with rationale must compile: %v", err)
	}
	for _, s := range art.Sections {
		if s.Key == "persona" && (s.Action != "dropped" || s.Rationale == "") {
			t.Fatalf("rationale must be recorded in artifact; got %+v", s)
		}
	}
}

func TestInvariantIncompleteSectionReportFailsClosed(t *testing.T) {
	failClosed(t, "section-report-complete", func(_ context.Context, in OptimizeInput) (OptimizedPrompt, error) {
		res, _ := identityFake(context.Background(), in)
		res.SectionOutcomes = res.SectionOutcomes[:len(res.SectionOutcomes)-1]
		return res, nil
	})
}

func TestInvariantUnknownSectionKeyFailsClosed(t *testing.T) {
	failClosed(t, "section-report-complete", func(_ context.Context, in OptimizeInput) (OptimizedPrompt, error) {
		res, _ := identityFake(context.Background(), in)
		res.SectionOutcomes = append(res.SectionOutcomes, SectionOutcome{Key: "ghost", Action: ActionPreserved})
		return res, nil
	})
}

func TestInvariantTokenRatchetFailsClosed(t *testing.T) {
	failClosed(t, "token-ratchet", func(_ context.Context, in OptimizeInput) (OptimizedPrompt, error) {
		res, _ := identityFake(context.Background(), in)
		res.Bytes = append(append([]byte{}, res.Bytes...), bytesRepeat("padding ", 64)...)
		return res, nil
	})
}

func TestInvariantTokenRatchetAllowGrowthPasses(t *testing.T) {
	dir := t.TempDir()
	a, cat := baseAssembler(t), baseCatalog(t)
	fn := func(_ context.Context, in OptimizeInput) (OptimizedPrompt, error) {
		res, _ := identityFake(context.Background(), in)
		res.Bytes = append(append([]byte{}, res.Bytes...), bytesRepeat("explicit growth ", 32)...)
		return res, nil
	}
	if _, err := Compile(context.Background(), a, nil, cat, VersionedOptimizer{Version: "grow/1", Fn: fn}, InvariantsContract{AllowGrowth: true}, dir); err != nil {
		t.Fatalf("AllowGrowth must bypass the ratchet explicitly: %v", err)
	}
}

func TestInvariantEmptyOutputFailsClosed(t *testing.T) {
	failClosed(t, "non-empty-output", func(_ context.Context, in OptimizeInput) (OptimizedPrompt, error) {
		res, _ := identityFake(context.Background(), in)
		res.Bytes = nil
		return res, nil
	})
}

func TestCompileCacheSkipsOptimizerCall(t *testing.T) {
	dir := t.TempDir()
	a, cat := baseAssembler(t), baseCatalog(t)
	ctx := context.Background()

	calls := 0
	counting := VersionedOptimizer{Version: "counting/1", Fn: func(c context.Context, in OptimizeInput) (OptimizedPrompt, error) {
		calls++
		return identityFake(c, in)
	}}

	first, err := Compile(ctx, a, nil, cat, counting, InvariantsContract{}, dir)
	if err != nil {
		t.Fatalf("first Compile: %v", err)
	}
	if calls != 1 {
		t.Fatalf("optimizer must be called exactly once; got %d", calls)
	}
	second, err := Compile(ctx, a, nil, cat, counting, InvariantsContract{}, dir)
	if err != nil {
		t.Fatalf("second Compile: %v", err)
	}
	if calls != 1 {
		t.Fatalf("cached Compile must not call the optimizer; calls=%d", calls)
	}
	if first.Hash != second.Hash || string(first.Bytes) != string(second.Bytes) {
		t.Fatal("cached Compile must return the identical artifact")
	}
}

func TestCompileRejectsEmptyOptimizerVersion(t *testing.T) {
	dir := t.TempDir()
	a, cat := baseAssembler(t), baseCatalog(t)
	_, err := Compile(context.Background(), a, nil, cat, VersionedOptimizer{Version: "", Fn: identityFake}, InvariantsContract{}, dir)
	if err == nil {
		t.Fatal("empty optimizer version must be rejected (unversioned artifacts are unauditable)")
	}
}

func bytesRepeat(s string, n int) []byte {
	b := make([]byte, 0, len(s)*n)
	for i := 0; i < n; i++ {
		b = append(b, s...)
	}
	return b
}
