package prompt

import (
	"context"
	"strings"
	"testing"
)

func TestToolCatalogSortsAndValidates(t *testing.T) {
	cat, err := NewToolCatalog([]ToolSummary{
		{Name: "write", Description: "w"},
		{Name: "read", Description: "r"},
	})
	if err != nil {
		t.Fatalf("NewToolCatalog: %v", err)
	}
	names := cat.Names()
	if len(names) != 2 || names[0] != "read" || names[1] != "write" {
		t.Fatalf("catalog must sort by name; got %v", names)
	}

	if _, err := NewToolCatalog([]ToolSummary{{Name: "read"}, {Name: "read"}}); err == nil {
		t.Fatal("duplicate tool name must be rejected")
	}
	for _, bad := range []string{"", " lead", "trail ", "-dash"} {
		if _, err := NewToolCatalog([]ToolSummary{{Name: bad}}); err == nil {
			t.Fatalf("tool name %q must be rejected", bad)
		}
	}
}

// identityFake preserves every section verbatim and outputs the same bytes.
func identityFake(_ context.Context, in OptimizeInput) (OptimizedPrompt, error) {
	outcomes := make([]SectionOutcome, 0, len(in.Sections))
	for _, s := range in.Sections {
		outcomes = append(outcomes, SectionOutcome{Key: s.Key, Action: ActionPreserved})
	}
	return OptimizedPrompt{
		Bytes:           RenderSections(in.Sections),
		SectionOutcomes: outcomes,
		ReferencedTools: in.Catalog.Names(),
	}, nil
}

func TestDedupOptimizerMergesIdenticalSections(t *testing.T) {
	body := "Use the read tool to inspect files."
	a := NewAssembler()
	if err := a.Register(Section{Number: 100, Key: "tools.read.a", Owner: "core", Body: body}); err != nil {
		t.Fatal(err)
	}
	if err := a.Register(Section{Number: 110, Key: "tools.read.b", Owner: "overlay", Body: body, Required: true}); err != nil {
		t.Fatal(err)
	}

	sections := mustAssemble(t, a, nil)
	cat, err := NewToolCatalog([]ToolSummary{{Name: "read", Description: "read files"}})
	if err != nil {
		t.Fatal(err)
	}
	in := OptimizeInput{Sections: sections, Catalog: cat}
	res, err := Dedup.Fn(context.Background(), in)
	if err != nil {
		t.Fatalf("Dedup.Fn: %v", err)
	}

	if strings.Count(string(res.Bytes), "inspect files.") != 1 {
		t.Fatalf("dedup must emit exactly one copy of the duplicated body; got:\n%s", res.Bytes)
	}
	byKey := map[string]SectionOutcome{}
	for _, o := range res.SectionOutcomes {
		byKey[o.Key] = o
	}
	if byKey["tools.read.a"].Action != ActionPreserved {
		t.Fatalf("earlier duplicate must be preserved; got %+v", byKey["tools.read.a"])
	}
	merged := byKey["tools.read.b"]
	if merged.Action != ActionMerged {
		t.Fatalf("later duplicate must be merged; got %+v", merged)
	}
	if strings.TrimSpace(merged.Rationale) == "" {
		t.Fatal("merged Required section must carry a rationale")
	}
	if len(res.Bytes) >= len(RenderSections(sections)) {
		t.Fatal("dedup output must be strictly smaller than its input render")
	}
	// The fake declares tool references it actually kept in the output bytes.
	if len(res.ReferencedTools) != 1 || res.ReferencedTools[0] != "read" {
		t.Fatalf("dedup must declare kept tool references; got %v", res.ReferencedTools)
	}
	// Version authority lives in VersionedOptimizer (Dedup.Version); the
	// result struct deliberately has no version field to misreport —
	// enforced by the type, asserted by compilation of this very check.
}
