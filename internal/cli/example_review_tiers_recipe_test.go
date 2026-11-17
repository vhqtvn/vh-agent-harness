package cli

// example_review_tiers_recipe_test.go — validates the shipped commit-reviewer
// roster recipes through the REAL merged loader.
//
// The example at templates/examples/.opencode/repo-configs/review-tiers.json
// (embed-only, printed by `vh-agent-harness example`) documents four recipes:
// a model-only swap (no override file), the shipped default 3+1 (no override
// file), the 4+1 single-specialist showcase (the live body), and the minimal
// 2+1 (embedded as the copyable //_recipe_minimal_2plus1 doc key). These
// tests prove each advertised shape against the reference loader with the
// merge semantics adopters actually get: CascadeConfig.from_file(base,
// override) — objects deep-merged key-by-key, arrays replaced wholesale —
// followed by validate_roster, preflight_models, and select_lightweight_leaf.
//
// WHY A GO TEST DRIVES PYTHON: tier_cascade.py IS the real merged loader (the
// orchestrator prompt is the primary runtime; the module is the reference
// implementation whose refusal semantics run_cascade enforces mechanically).
// The rendered-twin pitfall forbids teaching the CORPUS python test
// (tier_cascade_test.py, which renders into adopter repos) to depend on
// repo-local embed paths like templates/examples/... — so the harness-side
// validation lives here, driving the EMBEDDED corpus copy (exactly the bytes
// a built binary ships) out of a checkout-independent temp dir. The test
// skips (does not fail) when python3 is absent on the host.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	corpus "github.com/vhqtvn/vh-agent-harness"
)

const reviewTiersExamplePath = ".opencode/repo-configs/review-tiers.json"

// embeddedCoreFile reads one file out of the embedded core corpus — the bytes
// a built binary ships to adopters, independent of checkout-local state.
func embeddedCoreFile(t *testing.T, rel string) []byte {
	t.Helper()
	body, err := fs.ReadFile(corpus.CoreFS, "templates/core/"+rel)
	if err != nil {
		t.Fatalf("read embedded core file %s: %v", rel, err)
	}
	return body
}

// TestExampleListsReviewTiersRecipe pins discoverability: the roster-override
// recipe appears in the no-arg `example` listing.
func TestExampleListsReviewTiersRecipe(t *testing.T) {
	cmd, buf := newOutCmd()
	if err := runExample(cmd, nil); err != nil {
		t.Fatalf("runExample: %v", err)
	}
	if !strings.Contains(buf.String(), reviewTiersExamplePath) {
		t.Errorf("example list must mention %s\n--- output ---\n%s", reviewTiersExamplePath, buf.String())
	}
}

// TestExamplePrintsReviewTiersRecipe pins the emit path: printing the recipe
// yields byte-identical embedded content (what an adopter redirects to create
// their override file).
func TestExamplePrintsReviewTiersRecipe(t *testing.T) {
	index, _, err := exampleIndex()
	if err != nil {
		t.Fatalf("exampleIndex: %v", err)
	}
	want, ok := index[reviewTiersExamplePath]
	if !ok {
		t.Fatalf("example %s not in the embedded index", reviewTiersExamplePath)
	}
	cmd, buf := newOutCmd()
	if err := runExample(cmd, []string{reviewTiersExamplePath}); err != nil {
		t.Fatalf("runExample: %v", err)
	}
	if !bytes.Equal(buf.Bytes(), want) {
		t.Errorf("printed example must be byte-identical to the embedded body (got %d bytes, want %d)", buf.Len(), len(want))
	}
}

// reviewTierDriver is the python driver executed against the embedded
// reference loader. It evaluates every advertised recipe through
// CascadeConfig.from_file (base + optional override), then runs the roster
// validator, the model preflight (unseeded and seeded), and the lightweight
// seat selector, emitting one JSON summary for Go-side assertion.
const reviewTierDriver = `
import json
import sys
from pathlib import Path

sys.path.insert(0, sys.argv[1])
import tier_cascade

BASE = sys.argv[2]
MODELS_ROOT = Path(sys.argv[3])
SCENARIOS = [
    ("default_3plus1", None),
    ("recipe_4plus1", sys.argv[4]),
    ("recipe_2plus1", sys.argv[5]),
]

def load(override_path):
    return tier_cascade.CascadeConfig.from_file(BASE, override_path)

out = {"scenarios": [], "preflight_empty": [], "preflight_seeded": []}

for name, override in SCENARIOS:
    cfg = load(override)
    warnings = tier_cascade.validate_roster(cfg)
    generalists, specialists = tier_cascade.roster_composition(cfg)
    out["scenarios"].append({
        "name": name,
        "active": tier_cascade.active_leaves(cfg),
        "generalists": generalists,
        "specialists": specialists,
        "lightweight": tier_cascade.select_lightweight_leaf(cfg),
        "warnings": len(warnings),
    })

# Readiness boundary, unseeded: with NO model files, every recipe must
# hard-refuse, and the refusal must name every offending ACTIVE seat
# (inactive seats are exempt from model preflight).
for name, override in SCENARIOS:
    try:
        tier_cascade.preflight_models(load(override), MODELS_ROOT)
        out["preflight_empty"].append({"name": name, "refused": False, "detail": ""})
    except tier_cascade.ModelPreflightError as exc:
        out["preflight_empty"].append({"name": name, "refused": True, "detail": str(exc)})

# Readiness boundary, seeded: with every seat that is active under ANY recipe
# seeded, every recipe must pass preflight.
model_dir = MODELS_ROOT / ".local" / "config" / "agent-model"
model_dir.mkdir(parents=True, exist_ok=True)
active_all = set()
for _, override in SCENARIOS:
    active_all.update(tier_cascade.active_leaves(load(override)))
for leaf in sorted(active_all):
    (model_dir / leaf).write_text("test-model\n")

for name, override in SCENARIOS:
    try:
        tier_cascade.preflight_models(load(override), MODELS_ROOT)
        out["preflight_seeded"].append({"name": name, "passed": True, "detail": ""})
    except tier_cascade.ModelPreflightError as exc:
        out["preflight_seeded"].append({"name": name, "passed": False, "detail": str(exc)})

print(json.dumps(out))
`

type reviewTierScenario struct {
	Name        string   `json:"name"`
	Active      []string `json:"active"`
	Generalists []string `json:"generalists"`
	Specialists []string `json:"specialists"`
	Lightweight string   `json:"lightweight"`
	Warnings    int      `json:"warnings"`
}

type reviewTierPreflight struct {
	Name    string `json:"name"`
	Refused bool   `json:"refused"`
	Passed  bool   `json:"passed"`
	Detail  string `json:"detail"`
}

type reviewTierDriverOutput struct {
	Scenarios       []reviewTierScenario  `json:"scenarios"`
	PreflightEmpty  []reviewTierPreflight `json:"preflight_empty"`
	PreflightSeeded []reviewTierPreflight `json:"preflight_seeded"`
}

// TestReviewTiersRecipesValidateThroughMergedLoader is the crux receipt: every
// advertised recipe — including the verbatim emitted body (4+1) and the
// embedded copyable object (2+1) — must load through the REAL merged loader
// (base + override via from_file), pass roster validation with the expected
// composition, refuse dispatch unseeded while naming every active seat, and
// pass model preflight once those seats are seeded.
func TestReviewTiersRecipesValidateThroughMergedLoader(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 not on PATH — cannot drive the reference merged loader (tier_cascade.py)")
	}

	index, _, err := exampleIndex()
	if err != nil {
		t.Fatalf("exampleIndex: %v", err)
	}
	body, ok := index[reviewTiersExamplePath]
	if !ok {
		t.Fatalf("example %s not in the embedded index", reviewTiersExamplePath)
	}

	// The emitted body must be valid JSON — an adopter redirects it verbatim.
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(body, &doc); err != nil {
		t.Fatalf("example body is not valid JSON: %v", err)
	}
	raw2plus1, ok := doc["//_recipe_minimal_2plus1"]
	if !ok {
		t.Fatalf("example body lacks the //_recipe_minimal_2plus1 doc key")
	}

	tmp := t.TempDir()
	tcPath := filepath.Join(tmp, "tier_cascade.py")
	basePath := filepath.Join(tmp, "review-tiers.base.json")
	override4plus1 := filepath.Join(tmp, "review-tiers.override.4plus1.json")
	override2plus1 := filepath.Join(tmp, "review-tiers.override.2plus1.json")
	driverPath := filepath.Join(tmp, "driver.py")
	modelsRoot := filepath.Join(tmp, "repo-root")

	// The reference loader and base config come from the EMBEDDED corpus —
	// the exact bytes a built binary ships.
	if err := os.WriteFile(tcPath, embeddedCoreFile(t, ".opencode/sys-scripts/tier_cascade.py"), 0o644); err != nil {
		t.Fatalf("write tier_cascade.py: %v", err)
	}
	if err := os.WriteFile(basePath, embeddedCoreFile(t, ".opencode/config/review-tiers.json"), 0o644); err != nil {
		t.Fatalf("write base config: %v", err)
	}
	// The 4+1 override is the emitted body VERBATIM (the adopter's path);
	// the 2+1 override is the embedded copyable object, verbatim too.
	if err := os.WriteFile(override4plus1, body, 0o644); err != nil {
		t.Fatalf("write 4+1 override: %v", err)
	}
	if err := os.WriteFile(override2plus1, raw2plus1, 0o644); err != nil {
		t.Fatalf("write 2+1 override: %v", err)
	}
	if err := os.WriteFile(driverPath, []byte(reviewTierDriver), 0o644); err != nil {
		t.Fatalf("write driver: %v", err)
	}

	cmd := exec.Command(python, driverPath, tmp, basePath, modelsRoot, override4plus1, override2plus1)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("driver failed: %v\n--- stderr ---\n%s", err, stderr.String())
	}

	var got reviewTierDriverOutput
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("driver stdout is not JSON: %v\n--- stdout ---\n%s", err, stdout.String())
	}

	seat := func(s string) string { return "commit-reviewer-" + s }
	expected := map[string]reviewTierScenario{
		// Shipped default, no override: a..d active; a specialist, b/c/d
		// generalists; lightweight picks b (first active generalist).
		"default_3plus1": {
			Active:      []string{seat("a"), seat("b"), seat("c"), seat("d")},
			Generalists: []string{seat("b"), seat("c"), seat("d")},
			Specialists: []string{seat("a")},
			Lightweight: seat("b"),
			Warnings:    0,
		},
		// Live body: premium tier active, specialist re-seated a->e; 4
		// generalists + 1 specialist (single specialist: no floor warning).
		"recipe_4plus1": {
			Active:      []string{seat("a"), seat("b"), seat("c"), seat("d"), seat("e")},
			Generalists: []string{seat("a"), seat("b"), seat("c"), seat("d")},
			Specialists: []string{seat("e")},
			Lightweight: seat("a"),
			Warnings:    0,
		},
		// Minimal 2+1: one active tier [a,b,c]; a specialist via base
		// leaf_roles; b/c the required >=2 generalists; lightweight b.
		"recipe_2plus1": {
			Active:      []string{seat("a"), seat("b"), seat("c")},
			Generalists: []string{seat("b"), seat("c")},
			Specialists: []string{seat("a")},
			Lightweight: seat("b"),
			Warnings:    0,
		},
	}

	if len(got.Scenarios) != len(expected) {
		t.Fatalf("expected %d scenarios, got %d: %+v", len(expected), len(got.Scenarios), got.Scenarios)
	}
	for _, sc := range got.Scenarios {
		want, ok := expected[sc.Name]
		if !ok {
			t.Fatalf("unexpected scenario %q in driver output", sc.Name)
		}
		want.Name = sc.Name // expected literals key by name; align before comparing
		if !reflect.DeepEqual(sc, want) {
			t.Errorf("scenario %q mismatch:\n got %+v\nwant %+v", sc.Name, sc, want)
		}
	}

	// Unseeded readiness: every recipe refuses and names EVERY active seat.
	for _, pf := range got.PreflightEmpty {
		if !pf.Refused {
			t.Errorf("preflight (unseeded) for %q must refuse, got pass", pf.Name)
			continue
		}
		want, ok := expected[pf.Name]
		if !ok {
			t.Fatalf("unexpected scenario %q in preflight output", pf.Name)
		}
		for _, leaf := range want.Active {
			if !strings.Contains(pf.Detail, leaf) {
				t.Errorf("preflight (unseeded) for %q must name active seat %s in detail: %s", pf.Name, leaf, pf.Detail)
			}
		}
	}

	// Seeded readiness: every recipe passes once its active seats have models.
	for _, pf := range got.PreflightSeeded {
		if !pf.Passed {
			t.Errorf("preflight (seeded) for %q must pass, got refusal: %s", pf.Name, pf.Detail)
		}
	}

	t.Logf("validated recipes through the real merged loader: %s", fmt.Sprintf("%q", scenarioNames(got.Scenarios)))
}

func scenarioNames(scs []reviewTierScenario) []string {
	names := make([]string, 0, len(scs))
	for _, sc := range scs {
		names = append(names, sc.Name)
	}
	return names
}
