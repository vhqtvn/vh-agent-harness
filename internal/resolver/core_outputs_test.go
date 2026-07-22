package resolver

import (
	"reflect"
	"testing"
)

// helper: build a small catalog with two CoreOutputs-declaring capabilities.
func twoOutputCatalog(t *testing.T) *Catalog {
	t.Helper()
	return newCatalog(nil,
		CapabilityManifest{
			ID:       "core/media-perception",
			Provides: []string{"media-perception"},
			CoreOutputs: []string{
				".opencode/agents/media-perception.md",
				".opencode/skills/media-perception/SKILL.md",
			},
		},
		CapabilityManifest{
			ID:       "acme/extra",
			Provides: []string{"extra-agent"},
			CoreOutputs: []string{
				".opencode/agents/extra-agent.md",
			},
		},
	)
}

func TestCompileCoreSelectionPlan_AllSelected(t *testing.T) {
	c := twoOutputCatalog(t)
	selected, err := Resolve([]string{"core/media-perception", "acme/extra"}, c)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	plan := CompileCoreSelectionPlan(c, selected)
	wantActive := map[string]bool{
		".opencode/agents/media-perception.md":       true,
		".opencode/skills/media-perception/SKILL.md": true,
		".opencode/agents/extra-agent.md":            true,
	}
	if !reflect.DeepEqual(plan.ActiveLivePaths, wantActive) {
		t.Errorf("ActiveLivePaths = %v, want %v", plan.ActiveLivePaths, wantActive)
	}
	if len(plan.InactiveLivePaths) != 0 {
		t.Errorf("InactiveLivePaths should be empty when all selected, got %v", plan.InactiveLivePaths)
	}
	// All-known is the union.
	if !reflect.DeepEqual(plan.AllKnownLivePaths, wantActive) {
		t.Errorf("AllKnownLivePaths = %v, want %v", plan.AllKnownLivePaths, wantActive)
	}
}

func TestCompileCoreSelectionPlan_NoneSelected(t *testing.T) {
	c := twoOutputCatalog(t)
	// Empty selection — both capabilities' outputs are inactive.
	selected, err := Resolve(nil, c)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	plan := CompileCoreSelectionPlan(c, selected)
	if len(plan.ActiveLivePaths) != 0 {
		t.Errorf("ActiveLivePaths should be empty when nothing selected, got %v", plan.ActiveLivePaths)
	}
	wantInactive := map[string]bool{
		".opencode/agents/media-perception.md":       true,
		".opencode/skills/media-perception/SKILL.md": true,
		".opencode/agents/extra-agent.md":            true,
	}
	if !reflect.DeepEqual(plan.InactiveLivePaths, wantInactive) {
		t.Errorf("InactiveLivePaths = %v, want %v", plan.InactiveLivePaths, wantInactive)
	}
}

func TestCompileCoreSelectionPlan_PartialSelection(t *testing.T) {
	c := twoOutputCatalog(t)
	// Only media-perception selected; acme/extra is inactive.
	selected, err := Resolve([]string{"core/media-perception"}, c)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	plan := CompileCoreSelectionPlan(c, selected)
	wantActive := map[string]bool{
		".opencode/agents/media-perception.md":       true,
		".opencode/skills/media-perception/SKILL.md": true,
	}
	if !reflect.DeepEqual(plan.ActiveLivePaths, wantActive) {
		t.Errorf("ActiveLivePaths = %v, want %v", plan.ActiveLivePaths, wantActive)
	}
	wantInactive := map[string]bool{
		".opencode/agents/extra-agent.md": true,
	}
	if !reflect.DeepEqual(plan.InactiveLivePaths, wantInactive) {
		t.Errorf("InactiveLivePaths = %v, want %v", plan.InactiveLivePaths, wantInactive)
	}
}

func TestCompileCoreSelectionPlan_NilSelected(t *testing.T) {
	// nil selected set — everything declared is inactive (defensive).
	c := twoOutputCatalog(t)
	plan := CompileCoreSelectionPlan(c, nil)
	if len(plan.ActiveLivePaths) != 0 {
		t.Errorf("ActiveLivePaths should be empty with nil selected, got %v", plan.ActiveLivePaths)
	}
	if len(plan.InactiveLivePaths) != 3 {
		t.Errorf("InactiveLivePaths should have 3 entries, got %d (%v)", len(plan.InactiveLivePaths), plan.InactiveLivePaths)
	}
}

func TestCompileCoreSelectionPlan_NilCatalog(t *testing.T) {
	// nil catalog — empty plan (defensive; never panics).
	plan := CompileCoreSelectionPlan(nil, nil)
	if !plan.Empty() {
		t.Errorf("nil catalog should yield empty plan, got %+v", plan)
	}
}

func TestCompileCoreSelectionPlan_NoCoreOutputs(t *testing.T) {
	// A catalog where no capability declares CoreOutputs (the pre-media-perception
	// world) yields an empty plan — the short-circuit path.
	c := newCatalog(nil,
		CapabilityManifest{ID: "core/debate", Provides: []string{"debate"}},
	)
	selected, err := Resolve([]string{"core/debate"}, c)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	plan := CompileCoreSelectionPlan(c, selected)
	if !plan.Empty() {
		t.Errorf("catalog without CoreOutputs should yield empty plan, got %+v", plan)
	}
}

func TestCompileCoreSelectionPlan_NeverReturnsNilMaps(t *testing.T) {
	// Callers range over the maps unconditionally, so they must never be nil.
	plan := CompileCoreSelectionPlan(nil, nil)
	if plan.ActiveLivePaths == nil || plan.AllKnownLivePaths == nil || plan.InactiveLivePaths == nil {
		t.Fatalf("plan maps must never be nil, got %+v", plan)
	}
}

func TestCompileCoreSelectionPlan_CoreCatalogMediaSelected(t *testing.T) {
	// Integration: the real CoreCatalog with media-perception selected.
	c := CoreCatalog()
	selected, err := Resolve([]string{"core/media-perception"}, c)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	plan := CompileCoreSelectionPlan(c, selected)
	wantActive := map[string]bool{
		".opencode/agents/media-perception.md":       true,
		".opencode/skills/media-perception/SKILL.md": true,
	}
	if !reflect.DeepEqual(plan.ActiveLivePaths, wantActive) {
		t.Errorf("ActiveLivePaths = %v, want %v", plan.ActiveLivePaths, wantActive)
	}
	if len(plan.InactiveLivePaths) != 0 {
		t.Errorf("InactiveLivePaths should be empty when media-perception selected, got %v", plan.InactiveLivePaths)
	}
}

func TestCompileCoreSelectionPlan_CoreCatalogMediaUnselected(t *testing.T) {
	// Integration: the real CoreCatalog with media-perception NOT selected
	// (the default install profile does not select it).
	c := CoreCatalog()
	selected, err := Resolve(nil, c)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	plan := CompileCoreSelectionPlan(c, selected)
	if len(plan.ActiveLivePaths) != 0 {
		t.Errorf("ActiveLivePaths should be empty when media-perception unselected, got %v", plan.ActiveLivePaths)
	}
	wantInactive := map[string]bool{
		".opencode/agents/media-perception.md":       true,
		".opencode/skills/media-perception/SKILL.md": true,
	}
	if !reflect.DeepEqual(plan.InactiveLivePaths, wantInactive) {
		t.Errorf("InactiveLivePaths = %v, want %v", plan.InactiveLivePaths, wantInactive)
	}
}
