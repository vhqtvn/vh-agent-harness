package cli

// cohort_test.go pins the O4 Slice-2 pre-write cohort-link refusal behaviors
// (researches/decisions/2026-11-12-eslink-o4-s2-s4-wiring-scope.md §B + §E,
// Decisions 1B/O3):
//
//   - refusal is ZERO-mutation (disk hashes unchanged across the refused run);
//   - --allow-broken-cohort converts the refusal to WARN and proceeds, with
//     preserved files kept byte-for-byte;
//   - the O3 boundary predicate works in BOTH directions (platform-endpoint
//     edge refuses; adopter-internal edge warns and proceeds);
//   - acceptOnePath's prospective single-substitution gate, using the gate-5
//     state-lib/collectDecisionItems skew as the reference case;
//   - Decision 1B: update --dry-run prints WOULD REFUSE + disclaimer and
//     exits 0 (nil error), writing nothing;
//   - the flag is one-shot and non-persisted.
//
// All fixtures are synthetic t.TempDir targets seeded by a real seam install;
// the eslink corpus is never mutated and its pinned corpus tests stay
// untouched (61/46/15 · 49 · 14/3/32 · 5 · 0).

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vhqtvn/vh-agent-harness/internal/originhash"
	"github.com/vhqtvn/vh-agent-harness/internal/ownership"
	"github.com/vhqtvn/vh-agent-harness/internal/substrate"
)

// ownershipEffectiveMapForCohortTest builds a minimal effective map from
// path→class pairs for the ActivePlatformPaths unit test.
func ownershipEffectiveMapForCohortTest(classes map[string]string) ownership.EffectiveMap {
	eff := ownership.EffectiveMap{}
	for p, c := range classes {
		eff[p] = ownership.EffectiveEntry{Class: ownership.Class(c), Provenance: "test"}
	}
	return eff
}

func newCohortTestClassifier(eff ownership.EffectiveMap) *substrate.Classifier {
	return substrate.NewClassifier(eff, nil)
}

// gate5SkewStateLib rewrites the live state-lib.js in root with the gate-5
// surgery (internal/eslink corpus_test.go TestGate5_MixedTreeSkewRequired):
// both collectDecisionItems export-list entries removed — the pre-cccf1b0
// surface an adopter preserving state-lib.js at that base would hold. The
// install-time origin hash still records the CURRENT staged bytes, so the
// path classifies as consumer-edit preserved and update must assess the
// RETAINED live bytes, which prove exactly one broken platform-bound edge
// (verify-task-registry.js requires collectDecisionItems).
func gate5SkewStateLib(t *testing.T, root string) []byte {
	t.Helper()
	const hub = ".opencode/scripts/state-lib.js"
	p := filepath.Join(root, filepath.FromSlash(hub))
	cur, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read live %s: %v", hub, err)
	}
	const exportLine = "    collectDecisionItems,\n"
	if n := strings.Count(string(cur), exportLine); n != 2 {
		t.Fatalf("hub export-list shape moved: %d occurrences of the export line, want 2 — re-derive the skew fixture", n)
	}
	older := strings.ReplaceAll(string(cur), exportLine, "")
	if werr := os.WriteFile(p, []byte(older), substrate.RenderWriteMode(p)); werr != nil {
		t.Fatalf("write skewed %s: %v", hub, werr)
	}
	return []byte(older)
}

// hashTreeRoot returns path→sha256 for every regular file under root (the
// zero-mutation oracle).
func hashTreeRoot(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, rerr := filepath.Rel(root, p)
		if rerr != nil {
			return nil
		}
		data, derr := os.ReadFile(p)
		if derr != nil {
			return nil
		}
		out[filepath.ToSlash(rel)] = originhash.Digest(data)
		return nil
	})
	if err != nil {
		t.Fatalf("hash walk %s: %v", root, err)
	}
	return out
}

// withUpdateCohortFlag sets updateAllowBrokenCohort for the test and restores it.
func withUpdateCohortFlag(t *testing.T, v bool) {
	t.Helper()
	saved := updateAllowBrokenCohort
	t.Cleanup(func() { updateAllowBrokenCohort = saved })
	updateAllowBrokenCohort = v
}

// runUpdateTargetStderr runs a live update against target non-interactively,
// capturing BOTH the stdout buffer and everything written to os.Stderr (where
// the cohort diagnostics print).
func runUpdateTargetStderr(t *testing.T, target string) (stdout, stderr string, err error) {
	t.Helper()
	savedTarget, savedTTY := updateTargetFlag, updateStdinIsTTY
	t.Cleanup(func() { updateTargetFlag, updateStdinIsTTY = savedTarget, savedTTY })
	updateTargetFlag = target
	updateStdinIsTTY = func() bool { return false }
	cmd, buf := newOutCmd()
	stderr = captureStderr(t, func() {
		err = runUpdate(cmd, []string{})
	})
	return buf.String(), stderr, err
}

// TestCohortRefusal_ZeroMutationAndContract is the crux: a gate-5 state-lib
// skew (preserved live bytes missing collectDecisionItems) makes a LIVE
// update refuse BEFORE any write — the process exits non-zero
// (errCohortRefusal), the full 6-element contract prints to stderr, and the
// target tree is byte-for-byte unchanged.
func TestCohortRefusal_ZeroMutationAndContract(t *testing.T) {
	root := t.TempDir()
	seamInstallInto(t, root)
	skew := gate5SkewStateLib(t, root)

	before := hashTreeRoot(t, root)
	stdout, stderr, err := runUpdateTargetStderr(t, root)
	after := hashTreeRoot(t, root)

	if err == nil {
		t.Fatalf("live update over gate-5 skew must refuse, got nil error (stdout=%q stderr=%q)", stdout, stderr)
	}
	if !strings.Contains(err.Error(), "cohort-link refusal") {
		t.Fatalf("refusal error must carry the cohort-link refusal sentinel, got: %v", err)
	}
	// The broken edge must be named: verify-task-registry.js requires
	// collectDecisionItems from state-lib.js.
	for _, want := range []string{
		"collectDecisionItems",
		".opencode/scripts/verify-task-registry.js",
		".opencode/scripts/state-lib.js",
		"REFUSING",
		"zero files were written",
	} {
		if !strings.Contains(stderr, want) {
			t.Errorf("refusal diagnostic must contain %q\n--- stderr ---\n%s", want, stderr)
		}
	}
	// 6-element contract markers.
	for _, want := range []string{
		"SHA-256 of assessed prospective bytes",
		"preservation confirmation",
		"vh-agent-harness docs preserved-managed-reconciliation",
		"does NOT suppress subsequent doctor failures",
		cohortRider,
		"sha256:",
	} {
		if !strings.Contains(stderr, want) {
			t.Errorf("refusal contract must contain %q\n--- stderr ---\n%s", want, stderr)
		}
	}
	// Recovery wording must NOT promise plain accept-platform always resolves.
	if strings.Contains(stderr, "accept-platform always") {
		t.Errorf("recovery wording must not promise plain accept-platform always resolves:\n%s", stderr)
	}
	if len(before) != len(after) {
		t.Fatalf("refusal mutated the tree: %d files before, %d after", len(before), len(after))
	}
	for p, h := range before {
		if after[p] != h {
			t.Fatalf("refusal mutated %s: %s -> %s", p, h, after[p])
		}
	}
	// The skewed hub bytes must be untouched.
	hub := filepath.Join(root, ".opencode/scripts/state-lib.js")
	got, rerr := os.ReadFile(hub)
	if rerr != nil || string(got) != string(skew) {
		t.Fatalf("skewed hub must be byte-for-byte retained after refusal")
	}
}

// TestCohortDryRun_WouldRefuse_Exit0 pins Decision 1B: under --dry-run the
// same skew prints WOULD REFUSE + the full contract + the disclaimer, writes
// nothing, and the command SUCCEEDS (exit 0) so preview pipelines can still
// inspect the staged diff.
func TestCohortDryRun_WouldRefuse_Exit0(t *testing.T) {
	root := t.TempDir()
	seamInstallInto(t, root)
	gate5SkewStateLib(t, root)

	withDryRun(t, true)
	before := hashTreeRoot(t, root)
	stdout, stderr, err := runUpdateTargetStderr(t, root)
	after := hashTreeRoot(t, root)

	if err != nil {
		t.Fatalf("dry-run must exit 0 (Decision 1B), got %v (stderr=%q)", err, stderr)
	}
	for _, want := range []string{
		"WOULD REFUSE",
		cohortDryRunDisclaimer,
		cohortRider,
		"collectDecisionItems",
		"SHA-256 of assessed prospective bytes",
	} {
		if !strings.Contains(stderr, want) {
			t.Errorf("would-refuse preview must contain %q\n--- stderr ---\n%s", want, stderr)
		}
	}
	if len(before) != len(after) {
		t.Fatalf("dry-run wrote files: %d before, %d after", len(before), len(after))
	}
	for p, h := range before {
		if after[p] != h {
			t.Fatalf("dry-run mutated %s", p)
		}
	}
	if stdout == "" {
		t.Errorf("dry-run plan preview must still print after WOULD REFUSE (stdout empty)")
	}
}

// TestCohortOverride_WarnProceedPreservesBytes pins the escape flag: with
// --allow-broken-cohort a proven platform-bound broken link prints ALL
// diagnostics as WARN (never "REFUSING"), the apply proceeds, and the
// preserved file keeps its edited bytes byte-for-byte.
func TestCohortOverride_WarnProceedPreservesBytes(t *testing.T) {
	root := t.TempDir()
	seamInstallInto(t, root)
	skew := gate5SkewStateLib(t, root)

	withUpdateCohortFlag(t, true)
	stdout, stderr, err := runUpdateTargetStderr(t, root)
	if err != nil {
		t.Fatalf("override update must proceed, got %v (stderr=%q)", err, stderr)
	}
	if strings.Contains(stderr, "REFUSING") {
		t.Errorf("override must not refuse:\n%s", stderr)
	}
	for _, want := range []string{
		"proceeding with --allow-broken-cohort",
		"printed as WARN",
		cohortRider,
		"collectDecisionItems",
		"does NOT suppress subsequent doctor failures",
		"retained byte-for-byte",
	} {
		if !strings.Contains(stderr, want) {
			t.Errorf("override WARN diagnostic must contain %q\n--- stderr ---\n%s", want, stderr)
		}
	}
	// The override framing proceeds into a live apply — it must NOT claim
	// invocation-wide zero writes (C-F1 regression guard).
	if strings.Contains(stderr, "this run wrote nothing") {
		t.Errorf("override diagnostic must not claim the run wrote nothing (it proceeds to write advancing files):\n%s", stderr)
	}
	// Preservation is byte-for-byte: the consumer edit stands.
	hub := filepath.Join(root, ".opencode/scripts/state-lib.js")
	got, rerr := os.ReadFile(hub)
	if rerr != nil || string(got) != string(skew) {
		t.Fatalf("override apply must preserve the edited hub byte-for-byte (clobbered=%v)", rerr == nil && string(got) != string(skew))
	}
	if !strings.Contains(stdout, "seam reconciled") {
		t.Errorf("override update must complete the reconcile, stdout=%q", stdout)
	}
}

// TestCohortFlagNotPersisted pins one-shot non-persistence: after an override
// proceed, no file under .vh-agent-harness/ (lineage, profile, origin store,
// state) records the flag in any spelling.
func TestCohortFlagNotPersisted(t *testing.T) {
	root := t.TempDir()
	seamInstallInto(t, root)
	gate5SkewStateLib(t, root)

	withUpdateCohortFlag(t, true)
	if _, _, err := runUpdateTargetStderr(t, root); err != nil {
		t.Fatalf("override update: %v", err)
	}
	err := filepath.WalkDir(filepath.Join(root, ".vh-agent-harness"), func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, rerr := os.ReadFile(p)
		if rerr != nil {
			return nil
		}
		for _, needle := range []string{"allow-broken-cohort", "allow_broken_cohort", "allowBrokenCohort"} {
			if strings.Contains(string(data), needle) {
				t.Errorf("flag persisted in %s (%q)", p, needle)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk .vh-agent-harness: %v", err)
	}
}

// TestCohortO3_AdopterInternalWarnProceeds pins the O3 boundary in the
// non-refusing direction: a proven broken edge whose BOTH endpoints are
// adopter-unmanaged files (not in the active platform set) is reported as a
// WARNING and the update proceeds.
func TestCohortO3_AdopterInternalWarnProceeds(t *testing.T) {
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

	stdout, stderr, err := runUpdateTargetStderr(t, root)
	if err != nil {
		t.Fatalf("adopter-internal broken edge must NOT refuse (O3), got %v (stderr=%q)", err, stderr)
	}
	if strings.Contains(stderr, "REFUSING") || strings.Contains(stderr, "WOULD REFUSE") {
		t.Errorf("adopted-internal edge must not refuse:\n%s", stderr)
	}
	for _, want := range []string{
		"neverExported",
		".opencode/scratch/a.js",
		"never a refusal basis",
		cohortRider,
	} {
		if !strings.Contains(stderr, want) {
			t.Errorf("adopter-internal WARN must contain %q\n--- stderr ---\n%s", want, stderr)
		}
	}
	if !strings.Contains(stdout, "seam reconciled") {
		t.Errorf("update must complete, stdout=%q", stdout)
	}
}

// TestCohortO3_PlatformEndpointRefuses pins the boundary in the refusing
// direction from the adopter side: an ADOPTER-authored unmanaged file with a
// proven-broken import of a CLOSED platform module (the adopter→platform
// direction O3 exists to cover) refuses.
func TestCohortO3_PlatformEndpointRefuses(t *testing.T) {
	root := t.TempDir()
	seamInstallInto(t, root)

	// Adopter file importing a symbol the CURRENT platform state-lib never
	// exports. Provider (state-lib.js) is active-platform → refuse.
	extra := filepath.Join(root, ".opencode/scripts/adopter-extra.js")
	if err := os.WriteFile(extra, []byte(
		"import { symbolNoPlatformVersionExports } from \"./state-lib.js\";\n"), 0o644); err != nil {
		t.Fatalf("write adopter-extra.js: %v", err)
	}

	before := hashTreeRoot(t, root)
	_, stderr, err := runUpdateTargetStderr(t, root)
	after := hashTreeRoot(t, root)

	if err == nil {
		t.Fatalf("adopter→platform broken edge must refuse (O3), got nil (stderr=%q)", stderr)
	}
	if !strings.Contains(stderr, "symbolNoPlatformVersionExports") {
		t.Errorf("refusal must name the broken symbol\n%s", stderr)
	}
	if len(before) != len(after) {
		t.Fatalf("refusal mutated the tree")
	}
}

// acceptFixture prepares a seam staging + origin store for direct
// acceptOnePath calls against root.
func acceptFixture(t *testing.T, root string) (*seamStaging, *originhash.Store) {
	t.Helper()
	ps, cleanup, err := prepareSeamStaging(root, installRenderAnswers(root), nil)
	if err != nil {
		t.Fatalf("prepareSeamStaging: %v", err)
	}
	t.Cleanup(cleanup)
	store, serr := originhash.Read(root)
	if serr != nil {
		t.Fatalf("origin store read: %v", serr)
	}
	if store == nil {
		store = originhash.New()
	}
	return ps, store
}

// TestAcceptOnePath_ProspectiveGate_SequentialRepair is the gate-5 reference
// case: on the plain state-lib skew, accepting state-lib.js substitutes the
// CURRENT staged bytes, which heal the only broken edge — the prospective
// scan is clean and the acceptance proceeds WITHOUT the flag (sequential
// repair works when the single substitution heals everything).
func TestAcceptOnePath_ProspectiveGate_SequentialRepair(t *testing.T) {
	root := t.TempDir()
	seamInstallInto(t, root)
	gate5SkewStateLib(t, root)

	ps, store := acceptFixture(t, root)
	stagedHub, err := os.ReadFile(filepath.Join(ps.staging, ".opencode/scripts/state-lib.js"))
	if err != nil {
		t.Fatalf("read staged hub: %v", err)
	}

	res := acceptOnePath(ps, store, root, ".opencode/scripts/state-lib.js", false, false)
	if !res.ok {
		t.Fatalf("plain sequential repair (single substitution heals the tree) must proceed without the flag, got reason=%q", res.reason)
	}
	live, rerr := os.ReadFile(filepath.Join(root, ".opencode/scripts/state-lib.js"))
	if rerr != nil || string(live) != string(stagedHub) {
		t.Fatalf("accepted hub must equal staged bytes byte-for-byte")
	}
}

// TestAcceptOnePath_ProspectiveGate_RefusesWhenTreeStillBroken pins the
// refusing arm: when the prospective tree (current bytes with ONLY the
// proposed path substituted) still carries a platform-bound broken link —
// here an adopter file importing a symbol no version of the provider exports
// — the acceptance refuses with ZERO writes; --allow-broken-cohort escapes.
func TestAcceptOnePath_ProspectiveGate_RefusesWhenTreeStillBroken(t *testing.T) {
	root := t.TempDir()
	seamInstallInto(t, root)
	skew := gate5SkewStateLib(t, root)

	// An adopter-unmanaged importer whose required symbol is absent from
	// EVERY version of the platform provider: substituting the staged hub
	// heals the gate-5 edge but NOT this one, so the prospective tree stays
	// broken on a platform endpoint (state-lib.js).
	extra := filepath.Join(root, ".opencode/scripts/adopter-extra.js")
	if err := os.WriteFile(extra, []byte(
		"import { symbolNoPlatformVersionExports } from \"./state-lib.js\";\n"), 0o644); err != nil {
		t.Fatalf("write adopter-extra.js: %v", err)
	}

	ps, store := acceptFixture(t, root)

	before := hashTreeRoot(t, root)
	var res acceptResult
	captured := captureStderr(t, func() {
		res = acceptOnePath(ps, store, root, ".opencode/scripts/state-lib.js", false, false)
	})
	after := hashTreeRoot(t, root)

	if res.ok {
		t.Fatalf("acceptance must refuse while the prospective tree stays broken, got ok reason=%q", res.reason)
	}
	if !strings.Contains(res.reason, "cohort-link refusal") {
		t.Errorf("rejection reason must name the cohort-link refusal: %q", res.reason)
	}
	for _, want := range []string{"REFUSING", cohortRider, "symbolNoPlatformVersionExports", "vh-agent-harness docs preserved-managed-reconciliation"} {
		if !strings.Contains(captured, want) {
			t.Errorf("acceptance refusal diagnostic must contain %q\n--- stderr ---\n%s", want, captured)
		}
	}
	if len(before) != len(after) {
		t.Fatalf("refused acceptance mutated the tree")
	}
	for p, h := range before {
		if after[p] != h {
			t.Fatalf("refused acceptance mutated %s", p)
		}
	}
	live, _ := os.ReadFile(filepath.Join(root, ".opencode/scripts/state-lib.js"))
	if string(live) != string(skew) {
		t.Fatalf("refused acceptance must leave the skewed hub untouched")
	}

	// Escape flag: same acceptance proceeds, writes staged bytes, advances origin.
	stagedHub, err := os.ReadFile(filepath.Join(ps.staging, ".opencode/scripts/state-lib.js"))
	if err != nil {
		t.Fatalf("read staged hub: %v", err)
	}
	var res2 acceptResult
	overrideOut := captureStderr(t, func() {
		res2 = acceptOnePath(ps, store, root, ".opencode/scripts/state-lib.js", false, true)
	})
	if !res2.ok {
		t.Fatalf("acceptance with --allow-broken-cohort must proceed, got reason=%q", res2.reason)
	}
	if !strings.Contains(overrideOut, "proceeding with --allow-broken-cohort") {
		t.Errorf("override acceptance must print the WARN diagnostic:\n%s", overrideOut)
	}
	live2, _ := os.ReadFile(filepath.Join(root, ".opencode/scripts/state-lib.js"))
	if string(live2) != string(stagedHub) {
		t.Fatalf("override acceptance must write staged platform bytes")
	}
}

// TestAcceptOnePath_DryRunPreviewsRefusal pins preview honesty (the B-F1/C-F2
// blockers) and the operator-granted Decision-1B parity flip: the cohort gate
// runs BEFORE the dry-run early return (never a would-accept for a
// live-refusing acceptance), and the would-refusal previews as WOULD REFUSE +
// full contract + the VERBATIM 1B disclaimer in a distinct wouldRefuse
// disposition — never the "would accept" outcome. The override+dry-run
// combination (the filed DEFER card's gap) is pinned here too: the flag
// converts the preview to a would-accept with the override WARN diagnostic.
func TestAcceptOnePath_DryRunPreviewsRefusal(t *testing.T) {
	root := t.TempDir()
	seamInstallInto(t, root)
	gate5SkewStateLib(t, root)

	extra := filepath.Join(root, ".opencode/scripts/adopter-extra.js")
	if err := os.WriteFile(extra, []byte(
		"import { symbolNoPlatformVersionExports } from \"./state-lib.js\";\n"), 0o644); err != nil {
		t.Fatalf("write adopter-extra.js: %v", err)
	}

	ps, store := acceptFixture(t, root)
	before := hashTreeRoot(t, root)
	var res acceptResult
	captured := captureStderr(t, func() {
		res = acceptOnePath(ps, store, root, ".opencode/scripts/state-lib.js", true, false)
	})
	after := hashTreeRoot(t, root)

	if res.ok {
		t.Fatalf("dry-run must NOT report a would-accept for a live-refusing acceptance, got ok reason=%q", res.reason)
	}
	if !res.wouldRefuse {
		t.Fatalf("cohort would-refusal preview must carry the distinct wouldRefuse disposition (exit-0 parity bucket), got reason=%q", res.reason)
	}
	if !strings.Contains(res.reason, "WOULD REFUSE") || !strings.Contains(res.reason, "cohort-link refusal") {
		t.Errorf("dry-run rejection reason must name the would-refuse: %q", res.reason)
	}
	for _, want := range []string{"WOULD REFUSE", cohortRider, "symbolNoPlatformVersionExports", cohortDryRunDisclaimer} {
		if !strings.Contains(captured, want) {
			t.Errorf("dry-run would-refuse diagnostic must contain %q\n--- stderr ---\n%s", want, captured)
		}
	}
	if len(before) != len(after) {
		t.Fatalf("dry-run wrote files: %d before, %d after", len(before), len(after))
	}
	for p, h := range before {
		if after[p] != h {
			t.Fatalf("dry-run mutated %s", p)
		}
	}

	// Override + dry-run (DEFER-card gap): the flag converts the preview to a
	// would-accept with the override WARN diagnostic; still zero writes.
	var res2 acceptResult
	overrideOut := captureStderr(t, func() {
		res2 = acceptOnePath(ps, store, root, ".opencode/scripts/state-lib.js", true, true)
	})
	after2 := hashTreeRoot(t, root)
	if !res2.ok || res2.wouldRefuse {
		t.Fatalf("override+dry-run must preview as a would-accept (ok=true, wouldRefuse=false), got ok=%v wouldRefuse=%v reason=%q", res2.ok, res2.wouldRefuse, res2.reason)
	}
	for _, want := range []string{"proceeding with --allow-broken-cohort", "dry-run preview; would accept", cohortRider} {
		if !strings.Contains(overrideOut, want) {
			t.Errorf("override+dry-run diagnostic must contain %q\n--- stderr ---\n%s", want, overrideOut)
		}
	}
	if len(before) != len(after2) {
		t.Fatalf("override+dry-run wrote files: %d before, %d after", len(before), len(after2))
	}
}

// TestAcceptPlatform_DryRunExit0Parity_RiderBoundary pins the flip's three
// riders end-to-end through the batch verb:
//
//   - rider 2: a cohort would-refusal previews in its OWN WOULD REFUSE
//     section — the "would accept" summary path stays empty — and the
//     command exits 0 (inspection completed, not repair succeeded);
//   - rider 1: the exit-0 exception is cohort-findings-only — an invalid
//     path in the SAME dry-run invocation still fails the command non-zero
//     (its existing failure category is untouched);
//   - rider 3: the verbatim 1B disclaimer appears on the would-refuse
//     branch.
func TestAcceptPlatform_DryRunExit0Parity_RiderBoundary(t *testing.T) {
	root := t.TempDir()
	seamInstallInto(t, root)
	gate5SkewStateLib(t, root)

	if err := os.WriteFile(filepath.Join(root, ".opencode/scripts/adopter-extra.js"), []byte(
		"import { symbolNoPlatformVersionExports } from \"./state-lib.js\";\n"), 0o644); err != nil {
		t.Fatalf("write adopter-extra.js: %v", err)
	}

	savedTarget, savedAll, savedDry := acceptPlatformTarget, acceptPlatformAll, acceptPlatformDryRun
	t.Cleanup(func() {
		acceptPlatformTarget, acceptPlatformAll, acceptPlatformDryRun = savedTarget, savedAll, savedDry
	})
	acceptPlatformTarget, acceptPlatformAll, acceptPlatformDryRun = root, false, true

	// Cohort would-refuse ONLY: exit 0, distinct WOULD REFUSE section, no
	// would-accept summary, verbatim disclaimer on stderr.
	cmd, buf := newOutCmd()
	var err error
	stderr := captureStderr(t, func() { err = runAcceptPlatform(cmd, []string{".opencode/scripts/state-lib.js"}) })
	if err != nil {
		t.Fatalf("dry-run with only a cohort would-refusal must exit 0 (Decision-1B parity), got %v (stdout=%q stderr=%q)", err, buf.String(), stderr)
	}
	if !strings.Contains(buf.String(), "WOULD REFUSE 1 path(s)") {
		t.Errorf("stdout must carry the distinct WOULD REFUSE section (rider 2):\n%s", buf.String())
	}
	if strings.Contains(buf.String(), "would accept 1 path") || strings.Contains(buf.String(), "accepted 1 path") {
		t.Errorf("would-accept summary must stay empty on a refusal preview (rider 2):\n%s", buf.String())
	}
	if !strings.Contains(stderr, cohortDryRunDisclaimer) {
		t.Errorf("would-refuse preview must print the verbatim 1B disclaimer (rider 3):\n%s", stderr)
	}

	// Rider 1 boundary: the SAME dry-run with an invalid path alongside the
	// would-refusing path still fails non-zero — the exit-0 exception is
	// cohort-findings-only, every other failure category keeps non-zero.
	cmd2, buf2 := newOutCmd()
	_ = captureStderr(t, func() {
		err = runAcceptPlatform(cmd2, []string{
			".opencode/scripts/does-not-exist.js",
			".opencode/scripts/state-lib.js",
		})
	})
	if err == nil {
		t.Fatalf("dry-run with an invalid path must STILL exit non-zero (rider 1), got nil (stdout=%q)", buf2.String())
	}
	if !strings.Contains(buf2.String(), "NOT accepted") || !strings.Contains(buf2.String(), ".opencode/scripts/does-not-exist.js") {
		t.Errorf("invalid-path failure category must keep its existing NOT-accepted reporting (rider 1):\n%s", buf2.String())
	}
	if !strings.Contains(buf2.String(), "WOULD REFUSE 1 path(s)") {
		t.Errorf("the cohort would-refusal in the same invocation still previews distinctly:\n%s", buf2.String())
	}
	// F2 regression guard: with other errors present the WOULD REFUSE section
	// must NOT claim a blanket "preview completed with exit 0".
	if strings.Contains(buf2.String(), "preview completed with exit 0") {
		t.Errorf("mixed dry-run (refusal + hard error) must not claim blanket exit 0:\n%s", buf2.String())
	}
}

// TestAcceptPlatform_BatchEarlierLandLaterRefuse pins the C-F1 multi-path
// scenario: an EARLIER acceptance whose prospective tree is clean lands its
// write; a LATER acceptance refuses because ITS OWN substitution would break
// a platform-bound edge (substituting the staged hub drops a consumer-added
// export an adopter file imports). The later refusal must claim only that
// THIS acceptance wrote nothing — never an invocation-wide zero-write claim.
// It also pins C-F3 (non-JS preserved files appear in the preservation
// confirmation) and D-F2 (the substitute path is not listed as preserved).
func TestAcceptPlatform_BatchEarlierLandLaterRefuse(t *testing.T) {
	root := t.TempDir()
	seamInstallInto(t, root)

	// Consumer ADDS an export to the live hub (append-only edit, stays FIT).
	hubPath := filepath.Join(root, ".opencode/scripts/state-lib.js")
	hub, err := os.ReadFile(hubPath)
	if err != nil {
		t.Fatalf("read hub: %v", err)
	}
	edited := append([]byte(nil), hub...)
	edited = append(edited, []byte("export const adopterOnlySymbol = 1;\n")...)
	if err := os.WriteFile(hubPath, edited, 0o644); err != nil {
		t.Fatalf("write edited hub: %v", err)
	}
	// An adopter file importing the consumer-added export: the live tree is
	// linked, but substituting the STAGED hub (which lacks the symbol)
	// breaks the edge — the acceptance itself would break the cohort.
	if err := os.WriteFile(filepath.Join(root, ".opencode/scripts/adopter-extra.js"), []byte(
		"import { adopterOnlySymbol } from \"./state-lib.js\";\n"), 0o644); err != nil {
		t.Fatalf("write adopter-extra.js: %v", err)
	}
	// A second, independent consumer edit: its acceptance is clean (the
	// edited hub stays live in ITS prospective tree) and lands FIRST.
	ssPath := filepath.Join(root, ".opencode/plugins/session-state.js")
	ss, err := os.ReadFile(ssPath)
	if err != nil {
		t.Fatalf("read session-state: %v", err)
	}
	if err := os.WriteFile(ssPath, append(append([]byte(nil), ss...), []byte("// consumer edit\n")...), 0o644); err != nil {
		t.Fatalf("write edited session-state: %v", err)
	}
	// A NON-JS consumer-edited platform file: element 3 must enumerate it
	// even though only .js/.mjs modules are scanned/hashed (C-F3).
	agentPath := filepath.Join(root, ".opencode/agents/build.md")
	agent, err := os.ReadFile(agentPath)
	if err != nil {
		t.Fatalf("read agents/build.md: %v", err)
	}
	if err := os.WriteFile(agentPath, append(append([]byte(nil), agent...), []byte("\n<!-- consumer edit -->\n")...), 0o644); err != nil {
		t.Fatalf("write edited agents/build.md: %v", err)
	}

	// Drive the batch verb: earlier clean acceptance lands, later acceptance
	// refuses (its substitution breaks the adopter edge).
	savedTarget, savedAll := acceptPlatformTarget, acceptPlatformAll
	t.Cleanup(func() { acceptPlatformTarget, acceptPlatformAll = savedTarget, savedAll })
	acceptPlatformTarget, acceptPlatformAll = root, false
	cmd, buf := newOutCmd()
	var runErr error
	stderr := captureStderr(t, func() {
		runErr = runAcceptPlatform(cmd, []string{
			".opencode/plugins/session-state.js",
			".opencode/scripts/state-lib.js",
		})
	})

	if runErr == nil {
		t.Fatalf("batch with a refusing later path must exit non-zero (stdout=%q stderr=%q)", buf.String(), stderr)
	}
	// The EARLIER acceptance landed: live session-state == staged bytes.
	ps, cleanup, perr := prepareSeamStaging(root, installRenderAnswers(root), nil)
	if perr != nil {
		t.Fatalf("prepareSeamStaging: %v", perr)
	}
	defer cleanup()
	stagedSS, err := os.ReadFile(filepath.Join(ps.staging, ".opencode/plugins/session-state.js"))
	if err != nil {
		t.Fatalf("read staged session-state: %v", err)
	}
	liveSS, _ := os.ReadFile(ssPath)
	if string(liveSS) != string(stagedSS) {
		t.Fatalf("earlier clean acceptance must have landed its staged bytes")
	}
	// The LATER acceptance left the edited hub untouched.
	liveHub, _ := os.ReadFile(hubPath)
	if string(liveHub) != string(edited) {
		t.Fatalf("refused later acceptance must leave the edited hub untouched")
	}
	// The later refusal's diagnostic: per-path claim only (never
	// invocation-wide "this run wrote nothing"), the non-JS preserved file
	// enumerated (C-F3), and the substitute path NOT listed as a preserved
	// row (D-F2).
	if strings.Contains(stderr, "this run wrote nothing") {
		t.Errorf("later refusal must not claim the whole invocation wrote nothing (the earlier acceptance landed):\n%s", stderr)
	}
	if !strings.Contains(stderr, "per-path, not transactional") {
		t.Errorf("later refusal must scope its zero-write claim per path:\n%s", stderr)
	}
	if !strings.Contains(stderr, "[consumer-edit] .opencode/agents/build.md") {
		t.Errorf("preservation confirmation must enumerate the non-JS preserved file (C-F3):\n%s", stderr)
	}
	if strings.Contains(stderr, "[consumer-edit] .opencode/scripts/state-lib.js") {
		t.Errorf("the substitute path must not be listed as a preserved row (D-F2):\n%s", stderr)
	}
	if !strings.Contains(stderr, "adopterOnlySymbol") {
		t.Errorf("refusal must name the symbol its substitution would break:\n%s", stderr)
	}
}

// TestActivePlatformPaths_KeepsPlatformClassesOnly is the shared-helper unit
// pin: staged walk minus inactive, keeping {platform_managed,
// overlay_extension} only (armed/project-owned/unclassified staged files are
// excluded; inactive residue is excluded).
func TestActivePlatformPaths_KeepsPlatformClassesOnly(t *testing.T) {
	staging := t.TempDir()
	for _, rel := range []string{
		".opencode/scripts/hub.js",
		".opencode/plugins/ext.js",
		".opencode/repo-configs/note.md",
		".vh-agent-harness/vh-harness-profile.yml",
		"CLAUDE.md",
		".opencode/scripts/residue.js",
	} {
		p := filepath.Join(staging, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(p, []byte("export const x = 1;\n"), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
	eff := ownershipEffectiveMapForCohortTest(map[string]string{
		".opencode/scripts/hub.js":                 "platform_managed",
		".opencode/scripts/residue.js":             "platform_managed",
		".opencode/plugins/ext.js":                 "overlay_extension",
		".opencode/repo-configs/note.md":           "project_owned",
		".vh-agent-harness/vh-harness-profile.yml": "platform_armed",
	})
	cls := newCohortTestClassifier(eff)
	active := ActivePlatformPaths(staging, map[string]bool{".opencode/scripts/residue.js": true}, cls)
	for _, want := range []string{".opencode/scripts/hub.js", ".opencode/plugins/ext.js"} {
		if !active[want] {
			t.Errorf("active set must contain %s, got %v", want, active)
		}
	}
	for _, notWant := range []string{
		".opencode/scripts/residue.js",             // inactive residue excluded
		".opencode/repo-configs/note.md",           // project_owned excluded
		".vh-agent-harness/vh-harness-profile.yml", // armed excluded
		"CLAUDE.md", // project_owned (unclassified here) excluded
	} {
		if active[notWant] {
			t.Errorf("active set must NOT contain %s, got %v", notWant, active)
		}
	}
}
