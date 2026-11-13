package cli

// cohort_honesty_test.go pins the O4 Slice-4 HONESTY CONTRACT of every
// cohort-link surface (researches/decisions/2026-11-12-eslink-o4-s2-s4-wiring-
// scope.md §D): what the messages may and may not CLAIM, independent of the
// behavioral refusal semantics already pinned in cohort_test.go (S2) and
// doctor_cohort_links_test.go (S3).
//
// The contract:
//
//   - the literal rider ("link integrity only; behavior of preserved files is
//     not verified") appears on EVERY cohort refusal, WARN block, and doctor
//     #28 FAIL/WARN detail — asserted against the ACTUAL formatter surfaces
//     (formatCohortDiagnostic / formatCohortWarnBlock / emitSeamCohort
//     Diagnostics / checkCohortLinks), never against copies;
//   - banned over-claim phrases are ABSENT from every cohort-surface message;
//   - the Decision-1B disclaimer prints VERBATIM (from the constant) on a
//     dry-run would-refuse across ALL THREE verbs (update + install via
//     seamApply, accept-platform via the refused bucket);
//   - --allow-broken-cohort is non-persistent (no flag spelling anywhere under
//     .vh-agent-harness/ after an override run) and NEVER suppresses doctor
//     #28 (the same finding reports before and after an override run);
//   - recovery wording never promises plain accept-platform always resolves an
//     update refusal, and the routing vocabulary matches the corrected
//     overlay guidance (89ea161: additive-only packs; same-path goes through
//     project_owned in harness-ownership.yml; runbook docs pointer);
//   - dry-run framings make MODE-ACCURATE preservation claims (S2-F3): a
//     preview writes nothing and never borrows live-write wording;
//   - 2026-11-13 vendor-noise amendment: the vendor aggregate line carries
//     the literal rider itself, contains no banned phrase, and hides nothing
//     (the unknown totals still count vendor findings; the vendor PROVEN
//     edge stays itemized — facts, not noise).
//
// Corpus pins stay frozen (61/46/15 · 49 · 14/3/32 · 5 · 0); all fixtures are
// synthetic t.TempDir targets seeded by a real seam install.

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vhqtvn/vh-agent-harness/internal/eslink"
)

// --- ratified literals -------------------------------------------------------

// TestCohortHonesty_RatifiedLiteralsPinned guards the constants themselves
// against drift: the rider, the Decision-1B disclaimer, and the docs pointer
// are RATIFIED sentences (decision record §B elements 4/6 and Decision 1).
// Every other test in this file asserts those constants appear on real
// surfaces; this test pins what the constants must say.
func TestCohortHonesty_RatifiedLiteralsPinned(t *testing.T) {
	if cohortRider != "link integrity only; behavior of preserved files is not verified" {
		t.Errorf("cohortRider drifted from the ratified literal: %q", cohortRider)
	}
	if cohortDryRunDisclaimer != "Notice: dry-run exit 0 indicates preview completion only and does not certify live update readiness." {
		t.Errorf("cohortDryRunDisclaimer drifted from the ratified Decision-1B sentence: %q", cohortDryRunDisclaimer)
	}
	if cohortDocsPointer != "vh-agent-harness docs preserved-managed-reconciliation" {
		t.Errorf("cohortDocsPointer drifted from the runbook docs key: %q", cohortDocsPointer)
	}
}

// --- formatter surfaces (actual functions, never copies) ---------------------

// honestyCohortCheck is a synthetic verdict covering EVERY evidence class the
// diagnostic can carry (refuse edge, adopter-internal warn edge — including
// one under node_modules/ (2026-11-13 amendment: proven edges are never
// aggregated), unknown — non-vendor AND vendor (the aggregate-line class),
// hash row, preserved row) so one render exercises every message branch.
func honestyCohortCheck() *cohortCheck {
	return &cohortCheck{
		Refuse: []eslink.BrokenEdge{{
			Importer:  ".opencode/scripts/verify-task-registry.js",
			Specifier: "./state-lib.js",
			Provider:  ".opencode/scripts/state-lib.js",
			Symbol:    "collectDecisionItems",
			Reason:    "missing symbol collectDecisionItems",
		}},
		Warn: []eslink.BrokenEdge{
			{
				Importer:  ".opencode/scratch/a.js",
				Specifier: "./b.js",
				Provider:  ".opencode/scratch/b.js",
				Symbol:    "neverExported",
				Reason:    "missing symbol neverExported",
			},
			{
				// Vendor proven edge (the node_modules/yaml dogfood shape):
				// stays itemized everywhere — a fact, never aggregated.
				Importer:  ".opencode/node_modules/yaml/dist/cli.mjs",
				Specifier: "./util.mjs",
				Provider:  ".opencode/node_modules/yaml/dist/util.mjs",
				Symbol:    "isDocument",
				Reason:    "missing symbol isDocument",
			},
		},
		Unknown: []eslink.UnknownFinding{
			{
				Path:      ".opencode/scratch/dyn.js",
				Specifier: "./optional-panel.js",
				Reason:    "dynamic import",
			},
			{
				// Vendor unknowns (the aggregated class, 2026-11-13 posture
				// b+c): these paths must NEVER appear itemized on a surface.
				Path:      ".opencode/node_modules/effect/index.js",
				Specifier: "./lazy.js",
				Reason:    "dynamic import",
			},
			{
				Path:      ".opencode/node_modules/@ai-sdk/util/dist/helper.mjs",
				Specifier: "./lazy.js",
				Reason:    "dynamic import",
			},
		},
		Hashes: []cohortFileHash{{
			Path: ".opencode/scripts/state-lib.js",
			SHA:  "sha256:0000000000000000000000000000000000000000000000000000000000000000",
		}},
		Preserved: []cohortPreservedFile{{
			Path:   ".opencode/agents/build.md",
			Reason: "consumer-edit",
		}},
		Scanned: 3,
	}
}

// honestyAllClaimConstants enumerates EVERY element-3 preservation claim
// constant (live + dry-run-preview variants, both sites). When a new claim is
// added it MUST be registered here so the rider/banned-phrase sweep covers it.
func honestyAllClaimConstants() map[string]string {
	return map[string]string{
		"seamRefused":           cohortClaimSeamRefused,
		"seamOverride":          cohortClaimSeamOverride,
		"seamOverridePreview":   cohortClaimSeamOverridePreview,
		"acceptRefused":         cohortClaimAcceptRefused,
		"acceptRefusedPreview":  cohortClaimAcceptRefusedPreview,
		"acceptOverride":        cohortClaimAcceptOverride,
		"acceptOverridePreview": cohortClaimAcceptOverridePreview,
	}
}

// honestyDiagnosticSurfaces renders every full-diagnostic surface: formatter ×
// disposition mode × preservation claim, plus the WARN block and the emitter
// dispositions. Keyed by a descriptive name for failure messages.
func honestyDiagnosticSurfaces(t *testing.T) map[string]string {
	t.Helper()
	c := honestyCohortCheck()
	modes := []struct {
		mode cohortDiagMode
		name string
	}{
		{cohortModeRefuse, "refuse"},
		{cohortModeWouldRefuse, "would-refuse"},
		{cohortModeOverride, "override"},
	}
	out := map[string]string{}
	for claimName, claim := range honestyAllClaimConstants() {
		for _, m := range modes {
			out["diag/"+m.name+"/"+claimName] = formatCohortDiagnostic(c, m.mode, " (surface test)", claim)
		}
	}
	out["warn-block"] = formatCohortWarnBlock(c)

	// The seam emitter dispositions (the exact code paths seamApply calls).
	for _, tc := range []struct {
		name             string
		dryRun, override bool
	}{
		{"emit/refuse", false, false},
		{"emit/would-refuse", true, false},
		{"emit/override", false, true},
		{"emit/override-preview", true, true},
	} {
		var b bytes.Buffer
		emitSeamCohortDiagnostics(&b, c, tc.dryRun, tc.override)
		out[tc.name] = b.String()
	}
	return out
}

// cohortBannedPhrases are the over-claim vocabulary the cohort surfaces must
// NEVER use (decision §D + the O4 non-goal "no semantic claims"): the check
// proves link integrity only, so any phrasing implying behavior/semantics/
// logic was verified, or blanket compatibility, is a honesty defect.
var cohortBannedPhrases = []string{
	"semantic verification",
	"semantically verified",
	"semantically validated",
	"semantics verified",
	"behavior verified",
	"behaviorally sound",
	"semantically sound",
	"gate verified",
	"gate intact",
	"logic verified",
	"guaranteed compatible",
	"fully verified",
}

// TestCohortHonesty_RiderOnEveryFormatterSurface: the literal rider appears on
// EVERY refusal/warning rendering the formatter can emit.
func TestCohortHonesty_RiderOnEveryFormatterSurface(t *testing.T) {
	for name, s := range honestyDiagnosticSurfaces(t) {
		if !strings.Contains(s, cohortRider) {
			t.Errorf("surface %s must carry the literal rider:\n%s", name, s)
		}
	}
}

// TestCohortHonesty_BannedPhrasesAbsentFromFormatters: no formatter rendering
// contains any banned over-claim phrase (case-insensitive).
func TestCohortHonesty_BannedPhrasesAbsentFromFormatters(t *testing.T) {
	for name, s := range honestyDiagnosticSurfaces(t) {
		low := strings.ToLower(s)
		for _, banned := range cohortBannedPhrases {
			if strings.Contains(low, banned) {
				t.Errorf("surface %s contains banned phrase %q:\n%s", name, banned, s)
			}
		}
	}
}

// TestCohortHonesty_VendorAggregateLine (2026-11-13 amendment): the ONE
// aggregate line vendor unknown findings collapse to is itself honest — it
// carries the literal rider, contains no banned phrase, states the vendor
// count (never pretended zero), hides no TOTAL (the header count still
// includes vendor findings), itemizes no vendor unknown path, and never
// swallows the vendor PROVEN edge (facts stay itemized). Asserted against the
// actual formatter surfaces (which honestyCohortCheck now seeds with the
// vendor classes) and the live doctor WARN detail.
func TestCohortHonesty_VendorAggregateLine(t *testing.T) {
	aggCount := "+ 2 unknown finding(s) under vendor directories (node_modules/) not itemized"
	vendorUnknownPaths := []string{
		".opencode/node_modules/effect/index.js",
		".opencode/node_modules/@ai-sdk/util/dist/helper.mjs",
	}

	for name, s := range honestyDiagnosticSurfaces(t) {
		// The aggregate line exists, states its count, and carries the rider
		// ON THE LINE ITSELF (multi-line surfaces) / on the emitted detail
		// (single-line doctor surfaces).
		if !strings.Contains(s, aggCount) {
			t.Errorf("surface %s must carry the aggregate count line %q:\n%s", name, aggCount, s)
		}
		// Find the aggregate LINE and inspect exactly that line.
		var aggLine string
		for _, line := range strings.Split(s, "\n") {
			if strings.Contains(line, "under vendor directories") {
				aggLine = line
				break
			}
		}
		if aggLine == "" {
			t.Fatalf("surface %s: aggregate line not found:\n%s", name, s)
		}
		if !strings.Contains(aggLine, cohortRider) {
			t.Errorf("surface %s: aggregate line must carry the literal rider:\n%q", name, aggLine)
		}
		low := strings.ToLower(aggLine)
		for _, banned := range cohortBannedPhrases {
			if strings.Contains(low, banned) {
				t.Errorf("surface %s: aggregate line contains banned phrase %q:\n%q", name, banned, aggLine)
			}
		}
		// Honesty: vendor paths hidden from itemization but the TOTAL never
		// pretends zero — the surface's unknown header counts all 3 findings.
		for _, vp := range vendorUnknownPaths {
			if strings.Contains(s, vp) {
				t.Errorf("surface %s: vendor unknown path %s must not be itemized:\n%s", name, vp, s)
			}
		}
		if !strings.Contains(s, "unknown finding(s) (3") {
			t.Errorf("surface %s: unknown TOTAL must stay stated as 3 (1 itemized + 2 vendor):\n%s", name, s)
		}
		// The vendor PROVEN edge stays itemized — facts, not noise.
		for _, want := range []string{".opencode/node_modules/yaml/dist/cli.mjs", "isDocument"} {
			if !strings.Contains(s, want) {
				t.Errorf("surface %s: vendor proven edge must stay itemized; missing %s:\n%s", name, want, s)
			}
		}
	}

	// Live doctor WARN detail: a dogfood-shaped fixture (vendor unknowns via
	// dynamic imports) aggregates with the rider and hides no vendor path
	// from the COUNT while itemizing none of them.
	root := t.TempDir()
	seamInstallInto(t, root)
	vp := filepath.Join(root, ".opencode/node_modules/effect/index.js")
	if err := os.MkdirAll(filepath.Dir(vp), 0o755); err != nil {
		t.Fatalf("mkdir vendor dir: %v", err)
	}
	if err := os.WriteFile(vp, []byte(
		"const mod = await import(\"./lazy.js\");\nexport const ready = mod;\n"), 0o644); err != nil {
		t.Fatalf("write vendor module: %v", err)
	}
	r := checkCohortLinks(root)
	if r.tier != tierWarn {
		t.Fatalf("vendor unknown fixture must drive the WARN branch, got %s: %s", r.tier, r.detail)
	}
	if !strings.Contains(r.detail, "+ 1 unknown finding(s) under vendor directories (node_modules/) not itemized") {
		t.Errorf("doctor WARN detail must carry the vendor aggregate count:\n%s", r.detail)
	}
	if !strings.Contains(r.detail, cohortRider) {
		t.Errorf("doctor WARN detail must carry the literal rider:\n%s", r.detail)
	}
	if strings.Contains(r.detail, ".opencode/node_modules/effect") {
		t.Errorf("doctor WARN detail must not itemize the vendor unknown path:\n%s", r.detail)
	}
}

// TestCohortHonesty_DryRunClaimsModeAccurate pins the S2-F3 repair: the
// dry-run framings (seam override-preview via the emitter, and both preview
// claims by construction) state that the PREVIEW writes nothing and never
// borrow the live-write wording ("the proceeding ... writes"), which asserts a
// write a preview does not perform.
func TestCohortHonesty_DryRunClaimsModeAccurate(t *testing.T) {
	surfaces := honestyDiagnosticSurfaces(t)

	// The two preview claims + the emitter's override-preview disposition all
	// state the preview writes nothing.
	for _, name := range []string{
		"seamOverridePreview", "acceptRefusedPreview", "acceptOverridePreview",
	} {
		if !strings.Contains(honestyAllClaimConstants()[name], "write") ||
			!strings.Contains(strings.ToLower(honestyAllClaimConstants()[name]), "nothing") {
			t.Errorf("preview claim %s must state that nothing is written", name)
		}
	}
	if !strings.Contains(surfaces["emit/override-preview"], "dry-run preview") {
		t.Errorf("seam override under dry-run must be framed as a preview:\n%s", surfaces["emit/override-preview"])
	}
	if !strings.Contains(surfaces["emit/override-preview"], cohortClaimSeamOverridePreview) {
		t.Errorf("seam override under dry-run must use the preview claim, not the live-write claim:\n%s", surfaces["emit/override-preview"])
	}
	// The LIVE override framings keep their (accurate) live-write wording —
	// the preview wording must not leak into them either.
	for _, name := range []string{"emit/override", "diag/override/seamOverride", "diag/override/acceptOverride"} {
		if strings.Contains(surfaces[name], "dry-run preview writes nothing") {
			t.Errorf("live override surface %s must not carry preview wording", name)
		}
	}
	// Live-run landing facts ("already landed") belong ONLY to the live
	// accept-refused claim; no preview claim may assert them.
	for name, claim := range honestyAllClaimConstants() {
		if strings.Contains(claim, "already landed") && name != "acceptRefused" {
			t.Errorf("claim %s must not assert live-run landing facts: %q", name, claim)
		}
	}
}

// --- doctor #28 details (actual checkCohortLinks output) ---------------------

// TestCohortHonesty_DoctorDetailsRiderAndBannedPhrases drives the real check
// on fixtures: the FAIL detail (platform-bound proven edge), the WARN detail
// (unknown finding), and the inspection FAIL detail (non-directory .opencode)
// each carry the rider and contain no banned phrase.
func TestCohortHonesty_DoctorDetailsRiderAndBannedPhrases(t *testing.T) {
	details := map[string]string{}

	// FAIL: adopter→platform proven broken edge.
	root := t.TempDir()
	seamInstallInto(t, root)
	if err := os.WriteFile(filepath.Join(root, ".opencode/scripts/adopter-extra.js"), []byte(
		"import { symbolNoPlatformVersionExports } from \"./state-lib.js\";\n"), 0o644); err != nil {
		t.Fatalf("write adopter-extra.js: %v", err)
	}
	rFail := checkCohortLinks(root)
	if rFail.tier != tierFail {
		t.Fatalf("fixture must drive the FAIL branch, got %s: %s", rFail.tier, rFail.detail)
	}
	details["doctor/FAIL"] = rFail.detail

	// WARN: unknown finding (dynamic import) ONLY — the platform-bound edge
	// is removed so the check takes the WARN branch, not an early FAIL return.
	if err := os.Remove(filepath.Join(root, ".opencode/scripts/adopter-extra.js")); err != nil {
		t.Fatalf("remove adopter-extra.js: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".opencode/scratch"), 0o755); err != nil {
		t.Fatalf("mkdir scratch: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, ".opencode/scratch/dyn.js"), []byte(
		"const mod = await import(\"./optional-panel.js\");\nexport const ready = mod;\n"), 0o644); err != nil {
		t.Fatalf("write dyn.js: %v", err)
	}
	rWarn := checkCohortLinks(root)
	if rWarn.tier != tierWarn {
		t.Fatalf("fixture must drive the WARN branch, got %s: %s", rWarn.tier, rWarn.detail)
	}
	if !strings.Contains(rWarn.detail, "dynamic import") {
		t.Fatalf("WARN fixture must carry the dynamic-import finding:\n%s", rWarn.detail)
	}
	details["doctor/WARN"] = rWarn.detail

	// Inspection FAIL: .opencode exists but is not a directory.
	nd := t.TempDir()
	if err := os.WriteFile(filepath.Join(nd, ".opencode"), []byte("not a dir\n"), 0o644); err != nil {
		t.Fatalf("write .opencode file: %v", err)
	}
	rIns := checkCohortLinks(nd)
	if rIns.tier != tierFail {
		t.Fatalf("non-directory fixture must drive the inspection-FAIL branch, got %s: %s", rIns.tier, rIns.detail)
	}
	details["doctor/inspection-FAIL"] = rIns.detail

	for name, d := range details {
		if !strings.Contains(d, cohortRider) {
			t.Errorf("%s detail must carry the literal rider:\n%s", name, d)
		}
		low := strings.ToLower(d)
		for _, banned := range cohortBannedPhrases {
			if strings.Contains(low, banned) {
				t.Errorf("%s detail contains banned phrase %q:\n%s", name, banned, d)
			}
		}
	}
}

// TestCohortHonesty_AcceptPreviewClaimsAtEmissionSite pins the S2-F3 repair at
// the REAL emission sites (not just by injecting constants into the
// formatter): the dry-run refused and dry-run override framings acceptOnePath
// actually prints must carry the PREVIEW claims, never the live-write wording
// ("the proceeding acceptance writes ...") or live-run landing facts
// ("already landed").
func TestCohortHonesty_AcceptPreviewClaimsAtEmissionSite(t *testing.T) {
	root := t.TempDir()
	seamInstallInto(t, root)
	gate5SkewStateLib(t, root)
	if err := os.WriteFile(filepath.Join(root, ".opencode/scripts/adopter-extra.js"), []byte(
		"import { symbolNoPlatformVersionExports } from \"./state-lib.js\";\n"), 0o644); err != nil {
		t.Fatalf("write adopter-extra.js: %v", err)
	}
	ps, store := acceptFixture(t, root)
	before := hashTreeRoot(t, root)

	// Dry-run refused: the emitted element-3 claim is the preview claim.
	var res acceptResult
	refusedOut := captureStderr(t, func() {
		res = acceptOnePath(ps, store, root, ".opencode/scripts/state-lib.js", true, false)
	})
	if res.ok || !res.wouldRefuse {
		t.Fatalf("dry-run on a still-broken tree must land in the refused bucket, got ok=%v wouldRefuse=%v", res.ok, res.wouldRefuse)
	}
	if !strings.Contains(refusedOut, cohortClaimAcceptRefusedPreview) {
		t.Errorf("dry-run refusal must emit the PREVIEW claim at the emission site:\n%s", refusedOut)
	}
	for _, liveWording := range []string{"already landed", "the proceeding acceptance writes"} {
		if strings.Contains(refusedOut, liveWording) {
			t.Errorf("dry-run refusal must not carry live-write wording %q:\n%s", liveWording, refusedOut)
		}
	}

	// Dry-run override: preview (would-accept) claim, never the live claim.
	var res2 acceptResult
	overrideOut := captureStderr(t, func() {
		res2 = acceptOnePath(ps, store, root, ".opencode/scripts/state-lib.js", true, true)
	})
	if !res2.ok || res2.wouldRefuse {
		t.Fatalf("dry-run override must preview as a would-accept, got ok=%v wouldRefuse=%v", res2.ok, res2.wouldRefuse)
	}
	if !strings.Contains(overrideOut, cohortClaimAcceptOverridePreview) {
		t.Errorf("dry-run override must emit the PREVIEW claim at the emission site:\n%s", overrideOut)
	}
	if strings.Contains(overrideOut, cohortClaimAcceptOverride) || strings.Contains(overrideOut, "the proceeding acceptance writes") {
		t.Errorf("dry-run override must not carry the live-write claim:\n%s", overrideOut)
	}

	// Both previews wrote nothing.
	after := hashTreeRoot(t, root)
	if len(before) != len(after) {
		t.Fatalf("previews wrote files: %d before, %d after", len(before), len(after))
	}
	for p, h := range before {
		if after[p] != h {
			t.Fatalf("preview mutated %s", p)
		}
	}
}

// --- Decision 1B: verbatim disclaimer on would-refuse, all three verbs -------

// TestCohortHonesty_DisclaimerVerbatimAcrossThreeVerbs: a dry-run would-refuse
// prints the VERBATIM Decision-1B disclaimer (the constant, not a paraphrase)
// on update --dry-run and install --dry-run (both via seamApply) and on the
// accept-platform refused bucket (acceptOnePath dry-run), and each exits 0 /
// returns without refusal.
func TestCohortHonesty_DisclaimerVerbatimAcrossThreeVerbs(t *testing.T) {
	root := t.TempDir()
	seamInstallInto(t, root)
	gate5SkewStateLib(t, root)
	// The extra adopter edge keeps a single hub substitution from healing the
	// tree, so the accept verb's dry-run lands in the refused bucket too.
	if err := os.WriteFile(filepath.Join(root, ".opencode/scripts/adopter-extra.js"), []byte(
		"import { symbolNoPlatformVersionExports } from \"./state-lib.js\";\n"), 0o644); err != nil {
		t.Fatalf("write adopter-extra.js: %v", err)
	}

	// Verb 1: update --dry-run.
	withDryRun(t, true)
	_, stderr, err := runUpdateTargetStderr(t, root)
	if err != nil {
		t.Fatalf("update --dry-run must exit 0 on would-refuse (Decision 1B), got %v (stderr=%q)", err, stderr)
	}
	if !strings.Contains(stderr, cohortDryRunDisclaimer) {
		t.Errorf("update --dry-run would-refuse must print the VERBATIM 1B disclaimer:\n%s", stderr)
	}
	if !strings.Contains(stderr, "WOULD REFUSE") || !strings.Contains(stderr, cohortRider) {
		t.Errorf("update --dry-run would-refuse must carry WOULD REFUSE + rider:\n%s", stderr)
	}

	// Verb 2: install --dry-run (install-over-existing reaches the same
	// seamApply preflight; no selector keeps reconcile semantics).
	savedFl := installFl
	t.Cleanup(func() { installFl = savedFl })
	installFl = newInstallFlags()
	installFl.target = root
	installFl.dryRun = true
	cmd, buf := newOutCmd()
	instStderr := captureStderr(t, func() {
		err = runInstall(cmd, []string{})
	})
	if err != nil {
		t.Fatalf("install --dry-run must exit 0 on would-refuse (Decision 1B), got %v (stdout=%q stderr=%q)", err, buf.String(), instStderr)
	}
	if !strings.Contains(instStderr, cohortDryRunDisclaimer) {
		t.Errorf("install --dry-run would-refuse must print the VERBATIM 1B disclaimer:\n%s", instStderr)
	}

	// Verb 3: accept-platform dry-run, refused bucket.
	ps, store := acceptFixture(t, root)
	var res acceptResult
	accStderr := captureStderr(t, func() {
		res = acceptOnePath(ps, store, root, ".opencode/scripts/state-lib.js", true, false)
	})
	if res.ok || !res.wouldRefuse {
		t.Fatalf("accept dry-run on a still-broken tree must land in the refused bucket, got ok=%v wouldRefuse=%v reason=%q", res.ok, res.wouldRefuse, res.reason)
	}
	if !strings.Contains(accStderr, cohortDryRunDisclaimer) {
		t.Errorf("accept-platform dry-run refusal must print the VERBATIM 1B disclaimer:\n%s", accStderr)
	}
}

// --- escape flag: non-persistent + never suppresses doctor -------------------

// TestCohortHonesty_OverrideNonPersistentAndDoctorUnsuppressed: after an
// override run through the batch accept verb (which persists the origin store
// sidecar — the strongest persistence surface), no flag spelling exists
// anywhere under .vh-agent-harness/, and doctor #28 STILL reports the SAME
// finding the override bypassed (--allow-broken-cohort never suppresses the
// check).
func TestCohortHonesty_OverrideNonPersistentAndDoctorUnsuppressed(t *testing.T) {
	root := t.TempDir()
	seamInstallInto(t, root)
	gate5SkewStateLib(t, root)
	if err := os.WriteFile(filepath.Join(root, ".opencode/scripts/adopter-extra.js"), []byte(
		"import { symbolNoPlatformVersionExports } from \"./state-lib.js\";\n"), 0o644); err != nil {
		t.Fatalf("write adopter-extra.js: %v", err)
	}

	// Doctor BEFORE: the platform-bound edge FAILs and names the symbol.
	before := checkCohortLinks(root)
	if before.tier != tierFail || !strings.Contains(before.detail, "symbolNoPlatformVersionExports") {
		t.Fatalf("doctor #28 must FAIL naming the broken symbol before the override, got %s: %s", before.tier, before.detail)
	}

	// Override run through the batch verb (persists origin store + lineage).
	savedTarget, savedFlag := acceptPlatformTarget, acceptPlatformAllowBrokenCohort
	t.Cleanup(func() { acceptPlatformTarget, acceptPlatformAllowBrokenCohort = savedTarget, savedFlag })
	acceptPlatformTarget, acceptPlatformAllowBrokenCohort = root, true
	cmd, buf := newOutCmd()
	var runErr error
	overrideStderr := captureStderr(t, func() {
		runErr = runAcceptPlatform(cmd, []string{".opencode/scripts/state-lib.js"})
	})
	if runErr != nil {
		t.Fatalf("override acceptance must proceed, got %v (stdout=%q stderr=%q)", runErr, buf.String(), overrideStderr)
	}

	// Non-persistence: no flag spelling anywhere under .vh-agent-harness/.
	err := filepath.WalkDir(filepath.Join(root, ".vh-agent-harness"), func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, rerr := os.ReadFile(p)
		if rerr != nil {
			return nil
		}
		for _, needle := range []string{"allow-broken-cohort", "allow_broken_cohort", "allowBrokenCohort", "AllowBrokenCohort"} {
			if strings.Contains(string(data), needle) {
				t.Errorf("flag persisted in %s (%q)", p, needle)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk .vh-agent-harness: %v", err)
	}

	// Doctor AFTER: the SAME finding still reports (never suppressed).
	after := checkCohortLinks(root)
	if after.tier != tierFail {
		t.Fatalf("doctor #28 must STILL FAIL after the override run (never suppressed), got %s: %s", after.tier, after.detail)
	}
	if !strings.Contains(after.detail, "symbolNoPlatformVersionExports") {
		t.Errorf("doctor #28 must still name the same finding after the override:\n%s", after.detail)
	}
	if !strings.Contains(after.detail, cohortRider) {
		t.Errorf("doctor #28 post-override FAIL must still carry the rider:\n%s", after.detail)
	}
}

// --- recovery wording + corrected routing vocabulary --------------------------

// TestCohortHonesty_RecoveryWordingNeverPromisesAlwaysResolves: the recovery
// block in every full-diagnostic framing never promises that plain
// accept-platform always resolves an update refusal (the sequential-repair
// nuance: a single acceptance proceeds only when IT alone heals every
// platform-bound broken link).
func TestCohortHonesty_RecoveryWordingNeverPromisesAlwaysResolves(t *testing.T) {
	for name, s := range honestyDiagnosticSurfaces(t) {
		low := strings.ToLower(s)
		for _, banned := range []string{
			"accept-platform always",
			"always resolves",
			"will always fix",
		} {
			if strings.Contains(low, banned) {
				t.Errorf("surface %s promises resolution (%q):\n%s", name, banned, s)
			}
		}
		// "guaranteed to resolve" is admitted ONLY in its negated form (the
		// required honest bound); a bare affirmative is a promise.
		if stripped := strings.ReplaceAll(low, "not guaranteed to resolve", ""); strings.Contains(stripped, "guaranteed to resolve") {
			t.Errorf("surface %s asserts an un-negated resolution guarantee:\n%s", name, s)
		}
	}
	// The affirmative bound IS stated: a plain acceptance proceeds only when
	// the single substitution heals everything (so it is NOT guaranteed).
	any := honestyDiagnosticSurfaces(t)["diag/refuse/seamRefused"]
	if !strings.Contains(any, "NOT guaranteed to resolve") {
		t.Errorf("recovery block must state the not-guaranteed bound:\n%s", any)
	}
}

// TestCohortHonesty_RoutingVocabularyMatchesCorrectedOverlayGuidance: every
// full-diagnostic framing and the repaired driftRecoverySuffix carry the
// corrected routing vocabulary (89ea161): overlay packs are ADDITIVE-ONLY
// (new paths; a pack cannot shadow a builtin at the same path), same-path
// replacement goes through project_owned in harness-ownership.yml, and the
// runbook docs pointer is present. The refuted same-path overlay-promotion
// instruction is absent.
func TestCohortHonesty_RoutingVocabularyMatchesCorrectedOverlayGuidance(t *testing.T) {
	// The full diagnostic (recovery block, element 4) — check one live
	// framing; the block is shared by all framings.
	diag := formatCohortDiagnostic(honestyCohortCheck(), cohortModeRefuse, "", cohortClaimSeamRefused)
	for _, want := range []string{
		"vh-agent-harness docs preserved-managed-reconciliation",
		"NEW path",
		"cannot shadow a builtin at the same path",
		"project_owned",
		"harness-ownership.yml",
	} {
		if !strings.Contains(diag, want) {
			t.Errorf("diagnostic recovery block must contain %q:\n%s", want, diag)
		}
	}

	// driftRecoverySuffix — the fourth surface of the refuted cluster,
	// repaired in this slice. Assert through the formatter that embeds it.
	drift := formatManagedDriftFail("2 drifted, 0 missing of 50 managed",
		[]string{".opencode/a.md"}, nil)
	for _, want := range []string{
		".vh-agent-harness/overlays/<pack>/",
		"ADDITIONS only",
		"project_owned",
		"harness-ownership.yml",
		"vh-agent-harness docs preserved-managed-reconciliation",
	} {
		if !strings.Contains(drift, want) {
			t.Errorf("driftRecoverySuffix must contain %q:\n%s", want, drift)
		}
	}
	for _, refuted := range []string{
		"promote it into the overlay pack source", // same-path promotion
		"renders it as canonical content",         // the false promise
	} {
		if strings.Contains(drift, refuted) {
			t.Errorf("driftRecoverySuffix still carries the refuted route %q:\n%s", refuted, drift)
		}
	}
}
