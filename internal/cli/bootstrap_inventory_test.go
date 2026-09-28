package cli

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	corpus "github.com/vhqtvn/vh-agent-harness"
	"github.com/vhqtvn/vh-agent-harness/internal/overlay"
	"github.com/vhqtvn/vh-agent-harness/internal/resolver"
)

// ── BOOTSTRAP INVENTORY LEDGER (load-bearing policy surface) ─────────────────
//
// Every overlay pack and core capability shipped in the binary requires an
// EXPLICIT default-in / default-out decision, recorded HERE at introduction
// time. TestBootstrapInventory_Coverage fails (`go test ./internal/cli`, part
// of `make check`) until a decision exists for every shipped item: a new pack
// or capability with NO ledger entry cannot merge.
//
// Rules:
//   - decision "in"  → the item MUST appear in the frozen full recipe
//     (templates/install/full-harness-profile.yml) in the SAME change.
//   - decision "out" → the item must NOT appear in the recipe; it stays
//     available via explicit selection. A deliberate default-OUT is a valid,
//     passing state (see the synthetic subtests proving it).
//   - ledger entries for items that are no longer shipped are errors (remove
//     the entry when the item leaves the binary).
//   - duplicate entries are errors.
//
// Preset membership is pinned separately by TestBootstrapInventory_Presets.

// bootstrapInventoryDecision is one explicit default-in/default-out record.
type bootstrapInventoryDecision struct {
	name     string
	decision string // "in" | "out"
	reason   string
}

// bootstrapOverlayLedger is the decision ledger for shipped OVERLAY packs.
// Ground truth for "shipped" is overlay.KnownPacks() (the embedded
// templates/overlays directory). Project-local packs are NOT shipped inventory
// and never appear here.
var bootstrapOverlayLedger = []bootstrapInventoryDecision{
	{name: "auto-classifier-pilot", decision: "in", reason: "shipped safety pilot; part of the full surface"},
	{name: "contract-invariant-audit-pilot", decision: "in", reason: "default-on shipped pilot (INFORMS-only)"},
	{name: "formal-verification-pilot", decision: "in", reason: "default-on shipped pilot (INFORMS-only)"},
	{name: "frontend-ui-pilot", decision: "in", reason: "frontend-UI perception/integration pilot"},
	{name: "release", decision: "in", reason: "release ceremony pack; core/release capability"},
	{name: "repo-mail", decision: "in", reason: "inter-repo communication protocol overlay"},
	{name: "resolve-first-pilot", decision: "in", reason: "default-on shipped pilot (INFORMS-only)"},
}

// bootstrapCapabilityLedger is the decision ledger for shipped CORE
// CAPABILITIES. Ground truth for "shipped" is resolver.CoreCatalog() IDs UNION
// the capability manifests of embedded overlay packs (e.g. core/release from
// the release pack — catalog-only ground truth would miss it).
var bootstrapCapabilityLedger = []bootstrapInventoryDecision{
	{name: "core/debate", decision: "in", reason: "multi-model debate workflow"},
	{name: "core/gated-commit", decision: "in", reason: "gated-commit protocol"},
	{name: "core/media-perception", decision: "in", reason: "read-only perception specialist"},
	{name: "core/release", decision: "in", reason: "release specialist (via the release overlay pack)"},
	{name: "core/worker-read-only", decision: "in", reason: "prompt-scoped read-only worker"},
}

// validateBootstrapInventory checks bidirectional coverage between the SHIPPED
// inventory and the ledger, and that the frozen recipe's explicit selections
// equal exactly the `in` set. Pure: fixtures can be mutated for the synthetic
// future-exclusion subtests.
func validateBootstrapInventory(shippedOverlays, recipeOverlays []string, shippedCapabilities, recipeCapabilities []string, overlayLedger, capabilityLedger []bootstrapInventoryDecision) []error {
	var errs []error
	check := func(kind string, shipped, recipe []string, ledger []bootstrapInventoryDecision) {
		// Ledger integrity: duplicates are errors.
		seen := map[string]string{}
		for _, e := range ledger {
			if e.decision != "in" && e.decision != "out" {
				errs = append(errs, fmt.Errorf("%s ledger %q: invalid decision %q (want in|out)", kind, e.name, e.decision))
				continue
			}
			if prev, dup := seen[e.name]; dup {
				errs = append(errs, fmt.Errorf("%s ledger: duplicate decision for %q (%s and %s)", kind, e.name, prev, e.decision))
				continue
			}
			seen[e.name] = e.decision
		}
		// Every SHIPPED item has exactly one decision.
		shippedSet := map[string]bool{}
		for _, name := range shipped {
			shippedSet[name] = true
			if _, ok := seen[name]; !ok {
				errs = append(errs, fmt.Errorf("%s %q is shipped but has NO default-in/default-out decision; add a ledger entry in internal/cli/bootstrap_inventory_test.go (and add the item to templates/install/full-harness-profile.yml when the decision is `in`)", kind, name))
			}
		}
		// Ledger entries must reference shipped items (stale entries are errors).
		for _, e := range ledger {
			if !shippedSet[e.name] {
				errs = append(errs, fmt.Errorf("%s ledger entry %q references an item that is NOT shipped; remove the stale entry", kind, e.name))
			}
		}
		// The recipe's explicit list equals exactly the `in` set.
		want := map[string]bool{}
		for _, e := range ledger {
			if e.decision == "in" && shippedSet[e.name] {
				want[e.name] = true
			}
		}
		got := map[string]bool{}
		for _, name := range recipe {
			got[name] = true
			if !want[name] {
				errs = append(errs, fmt.Errorf("frozen full recipe selects %s %q whose ledger decision is NOT `in`; flip the ledger entry or remove it from the recipe", kind, name))
			}
		}
		for name := range want {
			if !got[name] {
				errs = append(errs, fmt.Errorf("frozen full recipe OMITS %s %q with an `in` decision; add it to templates/install/full-harness-profile.yml", kind, name))
			}
		}
	}
	check("overlay", shippedOverlays, recipeOverlays, overlayLedger)
	check("capability", shippedCapabilities, recipeCapabilities, capabilityLedger)
	return errs
}

// shippedOverlayNames returns the embedded overlay pack names (the binary's
// shipped pack inventory).
func shippedOverlayNames(t *testing.T) []string {
	t.Helper()
	names, err := overlay.KnownPacks()
	if err != nil {
		t.Fatalf("list embedded packs: %v", err)
	}
	return names
}

// shippedCapabilityIDs returns the shipped core capability IDs: CoreCatalog()
// UNION every embedded overlay-pack capability manifest (an empty temp target
// guarantees no project-local packs shadow the embedded set).
func shippedCapabilityIDs(t *testing.T) []string {
	t.Helper()
	contribs, err := discoverPackContributions(t.TempDir())
	if err != nil {
		t.Fatalf("discover embedded pack contributions: %v", err)
	}
	ids := map[string]bool{}
	for _, id := range resolver.CoreCatalog().IDs() {
		ids[id] = true
	}
	for _, c := range contribs {
		ids[c.Manifest.ID] = true
	}
	out := make([]string, 0, len(ids))
	for id := range ids {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

// recipeSelection loads the REAL frozen recipe install uses and returns its
// explicit (capabilities, overlays) lists.
func recipeSelection(t *testing.T) (capabilities, overlays []string) {
	t.Helper()
	raw, err := bootstrapRecipeBytes()
	if err != nil {
		t.Fatalf("load frozen full recipe: %v", err)
	}
	_, capabilities, overlays = parseBootstrapSelection(raw)
	return capabilities, overlays
}

// TestBootstrapInventory_Coverage is the build-time enforcement of the
// future-inclusion policy: every shipped overlay/capability carries exactly one
// explicit default-in/default-out decision, and the frozen full recipe's
// explicit selections equal the `in` set. A new pack without a decision FAILS
// this test (blocking `make check`) until the ledger entry is added.
func TestBootstrapInventory_Coverage(t *testing.T) {
	recipeCaps, recipeOverlays := recipeSelection(t)
	errs := validateBootstrapInventory(
		shippedOverlayNames(t), recipeOverlays,
		shippedCapabilityIDs(t), recipeCaps,
		bootstrapOverlayLedger, bootstrapCapabilityLedger)
	for _, err := range errs {
		t.Error(err)
	}
}

// TestBootstrapInventory_SyntheticNoDecisionFails proves the validator actually
// enforces the decision rule: a newly-shipped pack with NO ledger entry fails
// coverage (the "new pack ⇒ test fails until the ledger entry is added" crux).
func TestBootstrapInventory_SyntheticNoDecisionFails(t *testing.T) {
	// A future release ships a NEW pack (listed in KnownPacks) but nobody
	// recorded a decision for it. Coverage MUST fail naming the pack.
	shipped := append(shippedOverlayNames(t), "future-pack")
	recipeCaps, recipeOverlays := recipeSelection(t)
	errs := validateBootstrapInventory(
		shipped, recipeOverlays,
		shippedCapabilityIDs(t), recipeCaps,
		bootstrapOverlayLedger, bootstrapCapabilityLedger)
	if len(errs) == 0 {
		t.Fatalf("a shipped pack with NO ledger entry must FAIL coverage")
	}
	if !strings.Contains(errs[0].Error(), "future-pack") {
		t.Errorf("error must name the unhandled pack; got: %v", errs[0])
	}
}

// TestBootstrapInventory_SyntheticDefaultOutPasses proves a deliberate
// default-OUT is a valid, PASSING state: a shipped pack with an `out` decision
// and no recipe entry passes coverage. This keeps the ledger a genuine
// in/out decision surface — not an unconditional forever-full test that would
// defeat the default-out policy.
func TestBootstrapInventory_SyntheticDefaultOutPasses(t *testing.T) {
	overlays := append(shippedOverlayNames(t), "future-excluded-pack")
	ledger := append(bootstrapOverlayLedger, bootstrapInventoryDecision{
		name: "future-excluded-pack", decision: "out", reason: "deliberately default-out; select explicitly"})
	recipeCaps, recipeOverlays := recipeSelection(t)
	errs := validateBootstrapInventory(
		overlays, recipeOverlays,
		shippedCapabilityIDs(t), recipeCaps,
		ledger, bootstrapCapabilityLedger)
	for _, err := range errs {
		t.Errorf("a deliberate default-out decision must PASS coverage; got: %v", err)
	}
}

// TestBootstrapInventory_RecipeBytesAreEmbedded pins that the frozen recipe is
// served from corpus.InstallFS (binary-only embed, never a repo file): the
// accessor must succeed and the bytes must round-trip through the schema.
func TestBootstrapInventory_RecipeBytesAreEmbedded(t *testing.T) {
	raw, err := fs.ReadFile(corpus.InstallFS, filepath.Join(corpus.InstallDir, "full-harness-profile.yml"))
	if err != nil {
		t.Fatalf("read embedded recipe: %v", err)
	}
	if errs := validateBootstrapProfile(raw); len(errs) > 0 {
		t.Fatalf("embedded recipe is schema-invalid: %v", errs)
	}
	if !strings.Contains(string(raw), "profile: supervised") {
		t.Errorf("frozen recipe must carry the supervised preset")
	}
}

// TestBootstrapInventory_Presets pins the preset map's EXACT membership so
// existing presets cannot expand silently either: supervised is baseline +
// gated-commit + debate, and the other enum values are baseline-only. Widening
// a preset is a policy change that must be deliberate (this test flipping).
func TestBootstrapInventory_Presets(t *testing.T) {
	want := map[string][]resolver.CapabilityID{
		"minimal":      nil,
		"supervised":   {"core/gated-commit", "core/debate"},
		"coordination": nil,
		"web":          nil,
	}
	if len(profileCapabilityPresets) != len(want) {
		t.Errorf("preset enum changed (%d entries, want %d): %v — update this pin deliberately",
			len(profileCapabilityPresets), len(want), profileCapabilityPresets)
	}
	for name, caps := range want {
		got, ok := profileCapabilityPresets[name]
		if !ok {
			t.Errorf("preset %q missing from profileCapabilityPresets", name)
			continue
		}
		if len(got) != len(caps) {
			t.Errorf("preset %q = %v, want exactly %v (accidental preset expansion is a policy change)", name, got, caps)
			continue
		}
		for i := range caps {
			if got[i] != caps[i] {
				t.Errorf("preset %q = %v, want exactly %v", name, got, caps)
			}
		}
	}
}
