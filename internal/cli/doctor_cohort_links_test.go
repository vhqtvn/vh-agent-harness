package cli

// doctor_cohort_links_test.go pins doctor check #28 `cohort-links` (O4
// Slice 3, researches/decisions/2026-11-12-eslink-o4-s2-s4-wiring-scope.md
// §C + Decision 2/O3; vendor-noise reporting amendment
// researches/decisions/2026-11-13-eslink-o4-vendor-unknown-reporting-amendment.md):
//
//   - clean installed tree → PASS;
//   - proven broken edge touching the active platform set → FAIL naming
//     importer → provider, the missing symbol, and the reason, carrying the
//     literal rider, and driving a full doctor run to UNHEALTHY (exit 1);
//   - unknown findings (dynamic import) → WARN with the rider, doctor stays
//     HEALTHY (U6: unknown never FAILs);
//   - uninitialized directory (no .opencode tree) → SKIP;
//   - the O3 direction pair: platform-endpoint broken → FAIL vs
//     adopter-internal broken (both endpoints non-platform) → WARN, and the
//     preserved-platform-endpoint case (consumer-skewed state-lib.js) → FAIL
//     side (divergence is the FAIL side — the state-lib skew family this
//     check exists for);
//   - 2026-11-13 amendment: vendor-dir unknowns (node_modules/ path segment)
//     collapse to ONE aggregate line (count + rider) and are NOT itemized,
//     while a PROVEN edge under node_modules/ stays itemized (facts, never
//     aggregated), and a no-vendor tree renders NO aggregate line.
//
// All fixtures are synthetic t.TempDir targets seeded by a real seam install;
// the real repo corpus is never scanned and the eslink corpus pins stay
// untouched (61/46/15 · 49 · 14/3/32 · 5 · 0). No expected dogfood counts are
// hardcoded anywhere.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// cohortDoctorRun drives a full runDoctor against root (inside root as cwd,
// the seamDoctorOut precedent) and returns the report plus the returned error
// so tier tests can assert the UNHEALTHY/HEALTHY exit behavior of the check
// they provoked.
func cohortDoctorRun(t *testing.T, root string) (string, error) {
	t.Helper()
	var out string
	var err error
	runWithCwd(t, root, func() {
		doctorTargetFlag = root
		defer func() { doctorTargetFlag = "" }()
		cmd, buf := newOutCmd()
		err = runDoctor(cmd, []string{})
		out = buf.String()
	})
	return out, err
}

// TestCheckCohortLinks_PassCleanTree: a live .opencode/ JS tree with no
// broken edges and no unknown findings → PASS (0 broken, 0 unknown).
//
// The fixture is a minimal synthetic live tree, NOT a fresh full-recipe
// install: the shipped corpus itself carries the ratified known unknowns
// (update-op ++/-- debt + compaction-primitives — the documented dogfood
// noise), so no install shape can be PASS-clean until that debt is repaid.
// The membership oracle still runs the full staged render here; only the
// LIVE bytes (the scan input) are the minimal pair.
func TestCheckCohortLinks_PassCleanTree(t *testing.T) {
	root := t.TempDir()
	scratch := filepath.Join(root, ".opencode/scratch")
	if err := os.MkdirAll(scratch, 0o755); err != nil {
		t.Fatalf("mkdir scratch: %v", err)
	}
	if err := os.WriteFile(filepath.Join(scratch, "a.js"), []byte(
		"import { b } from \"./b.js\";\nexport const a = b;\n"), 0o644); err != nil {
		t.Fatalf("write a.js: %v", err)
	}
	if err := os.WriteFile(filepath.Join(scratch, "b.js"), []byte(
		"export const b = 1;\n"), 0o644); err != nil {
		t.Fatalf("write b.js: %v", err)
	}

	r := checkCohortLinks(root)
	if r.tier != tierPass {
		t.Fatalf("clean tree must PASS, got %s: %s", r.tier, r.detail)
	}
	if !strings.Contains(r.detail, "0 broken link(s), 0 unknown finding(s)") {
		t.Errorf("PASS detail must state the clean counts, got: %s", r.detail)
	}
}

// TestCheckCohortLinks_FailProvenBroken: an adopter-authored file with a
// proven-broken import of a CLOSED platform module (adopter→platform
// direction) FAILs, names the broken edge (importer → provider, missing
// symbol, reason), carries the literal rider and the G0c-relevant
// non-suppression caveat, and drives a full doctor run to UNHEALTHY.
func TestCheckCohortLinks_FailProvenBroken(t *testing.T) {
	root := t.TempDir()
	seamInstallInto(t, root)

	extra := filepath.Join(root, ".opencode/scripts/adopter-extra.js")
	if err := os.WriteFile(extra, []byte(
		"import { symbolNoPlatformVersionExports } from \"./state-lib.js\";\n"), 0o644); err != nil {
		t.Fatalf("write adopter-extra.js: %v", err)
	}

	// Unit tier: the check itself.
	r := checkCohortLinks(root)
	if r.tier != tierFail {
		t.Fatalf("platform-endpoint proven broken edge must FAIL, got %s: %s", r.tier, r.detail)
	}
	for _, want := range []string{
		".opencode/scripts/adopter-extra.js", // importer
		".opencode/scripts/state-lib.js",     // provider (active platform)
		"symbolNoPlatformVersionExports",     // missing symbol
		cohortRider,                          // literal scope rider
		"never suppresses this check",        // G0c/non-suppression caveat
	} {
		if !strings.Contains(r.detail, want) {
			t.Errorf("FAIL detail must contain %q, got:\n%s", want, r.detail)
		}
	}

	// Full-surface tier: doctor reports UNHEALTHY (exit-1 semantics — this is
	// what gates release G0c and adopter CI).
	out, err := cohortDoctorRun(t, root)
	if err == nil {
		t.Fatalf("doctor must exit non-zero on a cohort-links FAIL (G0c gate); output:\n%s", out)
	}
	sec := doctorSection(out, "cohort-links")
	if !strings.Contains(sec, "FAIL") {
		t.Errorf("doctor cohort-links section must FAIL; got:\n%s", sec)
	}
	if !strings.Contains(out, "result: UNHEALTHY") {
		t.Errorf("doctor must report UNHEALTHY; got:\n%s", out)
	}
}

// TestCheckCohortLinks_WarnUnknown: a dynamic import is an UNKNOWN finding —
// never a FAIL basis (U6). The check WARNs naming the finding and carrying
// the rider; a full doctor run stays HEALTHY (WARN never blocks).
func TestCheckCohortLinks_WarnUnknown(t *testing.T) {
	root := t.TempDir()
	seamInstallInto(t, root)

	scratch := filepath.Join(root, ".opencode/scratch")
	if err := os.MkdirAll(scratch, 0o755); err != nil {
		t.Fatalf("mkdir scratch: %v", err)
	}
	if err := os.WriteFile(filepath.Join(scratch, "dyn.js"), []byte(
		"const mod = await import(\"./optional-panel.js\");\nexport const ready = mod;\n"), 0o644); err != nil {
		t.Fatalf("write dyn.js: %v", err)
	}

	r := checkCohortLinks(root)
	if r.tier != tierWarn {
		t.Fatalf("unknown finding must WARN (never FAIL, U6), got %s: %s", r.tier, r.detail)
	}
	for _, want := range []string{
		".opencode/scratch/dyn.js",
		"dynamic import",
		"never a failure basis",
		cohortRider,
	} {
		if !strings.Contains(r.detail, want) {
			t.Errorf("WARN detail must contain %q, got:\n%s", want, r.detail)
		}
	}
	// A WARN detail must NOT carry the tier token "FAIL" — output-grepping
	// consumers (capability_output tests pin doctor output free of any
	// "FAIL" substring on clean installs) must not be tripped by prose.
	if strings.Contains(r.detail, "FAIL") {
		t.Errorf("WARN detail must not contain the tier token FAIL; got:\n%s", r.detail)
	}
	// 2026-11-13 amendment regression guard: a NO-vendor tree (this fixture's
	// only unknown sits under .opencode/scratch/) must render NO aggregate
	// line — the amendment changes nothing where there is no vendor noise.
	if strings.Contains(r.detail, "under vendor directories") {
		t.Errorf("no-vendor tree must not render the vendor aggregate line; got:\n%s", r.detail)
	}

	out, err := cohortDoctorRun(t, root)
	if err != nil {
		t.Fatalf("doctor must stay HEALTHY on a cohort-links WARN; got err=%v, output:\n%s", err, out)
	}
	if sec := doctorSection(out, "cohort-links"); !strings.Contains(sec, "WARN") {
		t.Errorf("doctor cohort-links section must WARN; got:\n%s", sec)
	}
	if !strings.Contains(out, "result: HEALTHY") {
		t.Errorf("doctor must report HEALTHY; got:\n%s", out)
	}
}

// TestCheckCohortLinks_SkipUninitialized: no .opencode tree → SKIP (doctor is
// meaningless before install; fail-open-safe like the greenfield arms of the
// neighboring checks).
func TestCheckCohortLinks_SkipUninitialized(t *testing.T) {
	root := t.TempDir()

	r := checkCohortLinks(root)
	if r.tier != tierSkip {
		t.Fatalf("uninitialized directory must SKIP, got %s: %s", r.tier, r.detail)
	}
}

// TestCheckCohortLinks_NotADirectoryFails: a .opencode that exists but is a
// FILE is an installed-but-unscannable state — an inspection FAIL carrying
// the rider, never a silent SKIP (review F2: only os.IsNotExist SKIPs).
func TestCheckCohortLinks_NotADirectoryFails(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".opencode"), []byte("not a dir\n"), 0o644); err != nil {
		t.Fatalf("write .opencode file: %v", err)
	}

	r := checkCohortLinks(root)
	if r.tier != tierFail {
		t.Fatalf("non-directory .opencode must FAIL (inspection failure), got %s: %s", r.tier, r.detail)
	}
	for _, want := range []string{"not a directory", cohortRider} {
		if !strings.Contains(r.detail, want) {
			t.Errorf("FAIL detail must contain %q, got:\n%s", want, r.detail)
		}
	}
}

// TestCheckCohortLinks_RenderOracleFailureFails: a membership-oracle render
// failure (malformed harness-ownership.yml aborts prepareSeamStaging) is a
// FAIL naming the oracle, carrying the rider — an operational FAIL obeys the
// same honesty discipline as a link-finding FAIL (review F1).
func TestCheckCohortLinks_RenderOracleFailureFails(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".opencode/scripts"), 0o755); err != nil {
		t.Fatalf("mkdir scripts: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".vh-agent-harness"), 0o755); err != nil {
		t.Fatalf("mkdir .vh-agent-harness: %v", err)
	}
	// Unparseable ownership overrides: prepareSeamStaging fails before any
	// write, so checkCohortLinks must surface the oracle failure.
	if err := os.WriteFile(filepath.Join(root, ".vh-agent-harness", "harness-ownership.yml"),
		[]byte("\tnot: [valid: yaml\n"), 0o644); err != nil {
		t.Fatalf("write harness-ownership.yml: %v", err)
	}

	r := checkCohortLinks(root)
	if r.tier != tierFail {
		t.Fatalf("membership-oracle render failure must FAIL, got %s: %s", r.tier, r.detail)
	}
	for _, want := range []string{"render staging for membership oracle", cohortRider} {
		if !strings.Contains(r.detail, want) {
			t.Errorf("FAIL detail must contain %q, got:\n%s", want, r.detail)
		}
	}
}

// TestCheckCohortLinks_O3_AdopterInternalWarns pins the boundary in the
// non-failing direction: a PROVEN broken edge whose BOTH endpoints are
// adopter-unmanaged files (not in the active platform set) is WARN-side only
// — enforcement never reaches into adopter-owned code (rejected alternative
// 2A), but the finding is still surfaced with the rider.
func TestCheckCohortLinks_O3_AdopterInternalWarns(t *testing.T) {
	root := t.TempDir()
	seamInstallInto(t, root)

	scratch := filepath.Join(root, ".opencode/scratch")
	if err := os.MkdirAll(scratch, 0o755); err != nil {
		t.Fatalf("mkdir scratch: %v", err)
	}
	if err := os.WriteFile(filepath.Join(scratch, "a.js"), []byte(
		"import { neverExported } from \"./b.js\";\n"), 0o644); err != nil {
		t.Fatalf("write a.js: %v", err)
	}
	if err := os.WriteFile(filepath.Join(scratch, "b.js"), []byte(
		"export const other = 1;\n"), 0o644); err != nil {
		t.Fatalf("write b.js: %v", err)
	}

	r := checkCohortLinks(root)
	if r.tier != tierWarn {
		t.Fatalf("adopter-internal proven broken edge must WARN (never FAIL), got %s: %s", r.tier, r.detail)
	}
	for _, want := range []string{
		".opencode/scratch/a.js",
		".opencode/scratch/b.js",
		"neverExported",
		cohortRider,
	} {
		if !strings.Contains(r.detail, want) {
			t.Errorf("WARN detail must contain %q, got:\n%s", want, r.detail)
		}
	}

	// Doctor stays HEALTHY: adopter-internal breakage never blocks.
	out, err := cohortDoctorRun(t, root)
	if err != nil {
		t.Fatalf("doctor must stay HEALTHY on adopter-internal WARN; got err=%v, output:\n%s", err, out)
	}
	if !strings.Contains(out, "result: HEALTHY") {
		t.Errorf("doctor must report HEALTHY; got:\n%s", out)
	}
}

// TestCheckCohortLinks_O3_PreservedPlatformEndpointFails pins the FAIL side
// of the preserved/diverged case — the state-lib skew family the whole O4 arc
// exists for: a consumer-skewed preserved state-lib.js (still in the active
// platform set — it WOULD render; divergence is the FAIL side) that no longer
// exports collectDecisionItems while the platform importer
// verify-task-registry.js still requires it, is a PROVEN platform-bound
// broken edge → FAIL.
func TestCheckCohortLinks_O3_PreservedPlatformEndpointFails(t *testing.T) {
	root := t.TempDir()
	seamInstallInto(t, root)
	gate5SkewStateLib(t, root)

	r := checkCohortLinks(root)
	if r.tier != tierFail {
		t.Fatalf("preserved-platform-endpoint proven broken edge must FAIL, got %s: %s", r.tier, r.detail)
	}
	for _, want := range []string{
		".opencode/scripts/verify-task-registry.js", // importer (platform)
		".opencode/scripts/state-lib.js",            // provider (preserved platform)
		"collectDecisionItems",                      // the missing symbol
		cohortRider,
	} {
		if !strings.Contains(r.detail, want) {
			t.Errorf("FAIL detail must contain %q, got:\n%s", want, r.detail)
		}
	}

	out, err := cohortDoctorRun(t, root)
	if err == nil {
		t.Fatalf("doctor must exit non-zero on the state-lib skew (the case this check exists for); output:\n%s", out)
	}
	if !strings.Contains(out, "result: UNHEALTHY") {
		t.Errorf("doctor must report UNHEALTHY; got:\n%s", out)
	}
}

// --- 2026-11-13 vendor-noise reporting amendment (posture "a+b+c") ----------

// TestIsVendorCohortPath_SegmentSemantics pins the deterministic vendor
// definition: any path CONTAINING a node_modules/ path SEGMENT (segment
// equality, never substring, never content sniffing).
func TestIsVendorCohortPath_SegmentSemantics(t *testing.T) {
	for _, tc := range []struct {
		rel  string
		want bool
	}{
		{".opencode/node_modules/effect/index.js", true},
		{".opencode/node_modules/@ai-sdk/util/dist/index.mjs", true},
		{".opencode/plugins/foo/node_modules/dep/lib.js", true},  // nested anywhere
		{".opencode/node_modules/a/node_modules/dep/b.js", true}, // repeated segments
		{".opencode/scripts/node_modules.js", false},             // file NAMED like the token
		{".opencode/scripts/node_modulesish/lib.js", false},      // prefix, not segment
		{".opencode/scratch/dyn.js", false},
		{".opencode/scripts/state-lib.js", false},
	} {
		if got := isVendorCohortPath(tc.rel); got != tc.want {
			t.Errorf("isVendorCohortPath(%q) = %v, want %v", tc.rel, got, tc.want)
		}
	}
}

// TestCheckCohortLinks_WarnVendorUnknownsAggregated pins the amendment on the
// live check surface with a dogfood-shaped fixture: a full seam install
// (carrying the known non-vendor platform unknowns) PLUS vendored files under
// .opencode/node_modules/ whose unknown findings must collapse to ONE
// aggregate line — present with the exact vendor count and the literal rider,
// while no vendor path is itemized and the non-vendor unknowns stay itemized.
// Tier stays WARN (posture (a): the WARN baseline, nothing suppressed) and
// doctor stays HEALTHY.
func TestCheckCohortLinks_WarnVendorUnknownsAggregated(t *testing.T) {
	root := t.TempDir()
	seamInstallInto(t, root)

	// Two vendored modules with unknown findings (dynamic imports — the same
	// finding class the dogfood vendor tree yields under the frozen grammar).
	for _, rel := range []string{
		".opencode/node_modules/effect/index.js",
		".opencode/node_modules/@ai-sdk/util/dist/helper.mjs",
	} {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", rel, err)
		}
		if err := os.WriteFile(p, []byte(
			"const mod = await import(\"./lazy.js\");\nexport const ready = mod;\n"), 0o644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}
	// One non-vendor unknown: must remain itemized.
	if err := os.MkdirAll(filepath.Join(root, ".opencode/scratch"), 0o755); err != nil {
		t.Fatalf("mkdir scratch: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, ".opencode/scratch/dyn.js"), []byte(
		"const mod = await import(\"./optional-panel.js\");\nexport const ready = mod;\n"), 0o644); err != nil {
		t.Fatalf("write dyn.js: %v", err)
	}

	r := checkCohortLinks(root)
	if r.tier != tierWarn {
		t.Fatalf("vendor unknown findings must keep the WARN baseline (posture a — no suppression, never FAIL), got %s: %s", r.tier, r.detail)
	}
	// The ONE aggregate line: exact vendor count (the fixture's two vendor
	// files are the only vendor modules) + the literal rider.
	agg := fmt.Sprintf("+ %d unknown finding(s) under vendor directories (node_modules/) not itemized", 2)
	if !strings.Contains(r.detail, agg) {
		t.Errorf("WARN detail must carry the aggregate line %q:\n%s", agg, r.detail)
	}
	if !strings.Contains(r.detail, "not itemized — "+cohortRider) {
		t.Errorf("the aggregate clause must carry the literal rider itself; got:\n%s", r.detail)
	}
	// No vendor path is itemized (posture (b)) — the aggregate line mentions
	// only the bare "(node_modules/)" token, never a .opencode/node_modules path.
	if strings.Contains(r.detail, ".opencode/node_modules") {
		t.Errorf("vendor unknown paths must NOT be itemized, only aggregated; got:\n%s", r.detail)
	}
	// Non-vendor unknowns stay itemized; totals still count everything
	// (posture (a)/(c): stated, never pretended zero).
	for _, want := range []string{
		".opencode/scratch/dyn.js",
		"unknown finding(s) (",
		cohortRider,
	} {
		if !strings.Contains(r.detail, want) {
			t.Errorf("WARN detail must contain %q, got:\n%s", want, r.detail)
		}
	}
	if !strings.Contains(r.detail, "never a failure basis") {
		t.Errorf("WARN detail must keep the U6 clause, got:\n%s", r.detail)
	}

	out, err := cohortDoctorRun(t, root)
	if err != nil {
		t.Fatalf("doctor must stay HEALTHY on a vendor-noise WARN; got err=%v, output:\n%s", err, out)
	}
	if !strings.Contains(out, "result: HEALTHY") {
		t.Errorf("doctor must report HEALTHY; got:\n%s", out)
	}
}

// TestCheckCohortLinks_VendorProvenEdgeStillItemized pins the first synthesis
// rider: a PROVEN broken edge is a fact, not noise — even with BOTH endpoints
// under node_modules/ (the exact node_modules/yaml dogfood shape) it stays
// itemized in the WARN details, never aggregated.
func TestCheckCohortLinks_VendorProvenEdgeStillItemized(t *testing.T) {
	root := t.TempDir()
	seamInstallInto(t, root)

	vendor := filepath.Join(root, ".opencode/node_modules/yaml/dist")
	if err := os.MkdirAll(vendor, 0o755); err != nil {
		t.Fatalf("mkdir yaml/dist: %v", err)
	}
	// dist/cli.mjs requires a symbol sibling build util.mjs does not export —
	// the real dogfood proven-edge shape (both endpoints adopter-internal:
	// node_modules never renders, so the edge is WARN-side).
	if err := os.WriteFile(filepath.Join(vendor, "cli.mjs"), []byte(
		"import { isDocument } from \"./util.mjs\";\nexport const cli = isDocument;\n"), 0o644); err != nil {
		t.Fatalf("write cli.mjs: %v", err)
	}
	if err := os.WriteFile(filepath.Join(vendor, "util.mjs"), []byte(
		"export const isNodeStream = 1;\n"), 0o644); err != nil {
		t.Fatalf("write util.mjs: %v", err)
	}

	r := checkCohortLinks(root)
	if r.tier != tierWarn {
		t.Fatalf("adopter-internal vendor proven edge must WARN (never FAIL), got %s: %s", r.tier, r.detail)
	}
	for _, want := range []string{
		".opencode/node_modules/yaml/dist/cli.mjs",  // importer — ITEMIZED
		".opencode/node_modules/yaml/dist/util.mjs", // provider — ITEMIZED
		"isDocument", // the missing symbol
		cohortRider,
	} {
		if !strings.Contains(r.detail, want) {
			t.Errorf("vendor proven edge must stay itemized (never aggregated); missing %q in:\n%s", want, r.detail)
		}
	}
}
