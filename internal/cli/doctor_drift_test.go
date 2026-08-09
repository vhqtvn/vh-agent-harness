package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	corpus "github.com/vhqtvn/vh-agent-harness"
	"github.com/vhqtvn/vh-agent-harness/internal/originhash"
	"github.com/vhqtvn/vh-agent-harness/internal/ownership"
)

// writeOwnershipOverrides writes a harness-ownership.yml under
// <target>/.vh-agent-harness/ raising each path->class. This is the S2 authority
// readOwnershipOverrides consumes. Path keys use repo-relative slash form.
func writeOwnershipOverrides(t *testing.T, target string, raises map[string]string) {
	t.Helper()
	dir := filepath.Join(target, ".vh-agent-harness")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	var b strings.Builder
	b.WriteString("overrides:\n")
	for p, c := range raises {
		b.WriteString("  ")
		b.WriteString(p)
		b.WriteString(":\n    class: ")
		b.WriteString(c)
		b.WriteString("\n    reason: \"test raise\"\n")
	}
	if err := os.WriteFile(filepath.Join(dir, ownershipOverridesFileName), []byte(b.String()), 0o644); err != nil {
		t.Fatalf("write ownership overrides: %v", err)
	}
}

// findLivePlatformManagedPath returns the repo-relative slash path of a corpus
// platform_managed path that exists on disk under root. Used to pick a robust
// fixture path that is independent of which files a given profile renders.
func findLivePlatformManagedPath(t *testing.T, root string) string {
	t.Helper()
	def, err := corpus.CoreOwnershipDefaults()
	if err != nil {
		t.Fatalf("core ownership defaults: %v", err)
	}
	for p, rule := range def {
		if rule.Class != ownership.ClassPlatformManaged {
			continue
		}
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(p))); err == nil {
			return p
		}
	}
	t.Fatalf("no live platform_managed path found under %s", root)
	return ""
}

// findLiveAuthoredPlatformManagedPath is like findLivePlatformManagedPath but
// EXCLUDES the platform-regenerated paths (regeneratedPlatformPaths, currently
// allowed-commands.js and opencode.jsonc). Use this for tests that need a managed path whose
// origin-hash three-way preservation actually applies (consumer edits/deletions
// update respects) — i.e. NOT a path the platform overwrites canonically every
// apply. The base helper can return a regenerated path (allowed-commands.js IS
// platform_managed in the corpus defaults), which would silently flip a test's
// expected preserved-vs-genuine verdict; this variant removes that flake.
func findLiveAuthoredPlatformManagedPath(t *testing.T, root string) string {
	t.Helper()
	def, err := corpus.CoreOwnershipDefaults()
	if err != nil {
		t.Fatalf("core ownership defaults: %v", err)
	}
	for p, rule := range def {
		if rule.Class != ownership.ClassPlatformManaged {
			continue
		}
		if regeneratedPlatformPaths[p] {
			continue
		}
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(p))); err == nil {
			return p
		}
	}
	t.Fatalf("no live authored (non-regenerated) platform_managed path found under %s", root)
	return ""
}

// findLiveDefaultNonManagedPath returns the repo-relative slash path of a corpus
// path whose DEFAULT ownership class is project_owned (e.g. a render-independent
// seed like README.md, Makefile, CLAUDE.md, .gitignore, docs/planning/backlog.md,
// docs/planning/roadmap.md, or forbidden-patterns.project.js) and that exists on
// disk under root. Used to exercise the F3 regression: a default-class non-managed
// file that diverges must be silently skipped — not labeled "project-preserved
// (ownership override)".
//
// Render-independence rationale (why this is restricted to ClassProjectOwned):
// checkManagedDrift's re-render reads SOME non-managed files as inputs. The
// platform_armed vh-harness-profile.yml is consumed by readProfileAnswers, and
// external_generated recon data is consumed by the recon loader. Corrupting
// either of those in the test would corrupt the re-render itself and surface an
// orthogonal drift failure (e.g. opencode.jsonc drifting because a profile seed
// was dropped) unrelated to the F3 assertion. ClassProjectOwned seeds are NOT
// read as render inputs, so corrupting one exercises exactly the F3 path
// (default-class divergence silently skipped) with no render-input coupling.
// platform_managed is excluded because it is the F3 in-scope class, not the
// default-non-managed case under test.
func findLiveDefaultNonManagedPath(t *testing.T, root string) string {
	t.Helper()
	def, err := corpus.CoreOwnershipDefaults()
	if err != nil {
		t.Fatalf("core ownership defaults: %v", err)
	}
	for p, rule := range def {
		if rule.Class != ownership.ClassProjectOwned {
			continue
		}
		// Exclude project_owned files that ARE read as render inputs —
		// corrupting those would break the re-render itself, surfacing an
		// orthogonal error rather than exercising the F3 skip path. The
		// permission transform (config-transform.mjs) is read by
		// applyConfigTransform inside renderSeamStaging.
		if p == ".vh-agent-harness/config-transform.mjs" {
			continue
		}
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(p))); err == nil {
			return p
		}
	}
	t.Fatalf("no live default non-managed path found under %s", root)
	return ""
}

// TestManagedDrift_NoOverride_Pass: a clean seam install with no
// harness-ownership.yml must report managed-drift as PASS with "in sync" detail.
// This is the unchanged-behavior baseline: the override-awareness path must be a
// no-op when no override file is present.
func TestManagedDrift_NoOverride_Pass(t *testing.T) {
	root := t.TempDir()
	seamInstallInto(t, root)

	r := checkManagedDrift(root)
	if r.tier != tierPass {
		t.Fatalf("want PASS for clean install (no overrides), got %s: %s", r.tier, r.detail)
	}
	if !strings.Contains(r.detail, "in sync") {
		t.Errorf("PASS detail should say 'in sync'; got %q", r.detail)
	}
	if strings.Contains(r.detail, "preserved") {
		t.Errorf("PASS detail should not mention preserved when no overrides; got %q", r.detail)
	}
}

// TestManagedDrift_NoOverride_NoOrigin_IsMigrationStalled: F6 behavior lock.
// A managed file with NO recorded origin (bootstrap/pre-feature) whose live
// bytes differ from a fresh render is NOT genuine drift — it is an
// unknown-baseline collision (the F6 adoption-migration gate). Doctor must
// surface it as non-failing migration-stalled INFO (update preserves the live
// bytes, never clobbers without origin proof), NOT as drift FAIL. The
// origin-hash store is removed to simulate the pre-feature state.
func TestManagedDrift_NoOverride_NoOrigin_IsMigrationStalled(t *testing.T) {
	root := t.TempDir()
	seamInstallInto(t, root)
	// Remove the origin-hash store (bootstrap/pre-feature semantics).
	if err := os.Remove(originhash.FilePath(root)); err != nil && !os.IsNotExist(err) {
		t.Fatalf("remove origin-hash store: %v", err)
	}
	p := findLivePlatformManagedPath(t, root)
	live := filepath.Join(root, filepath.FromSlash(p))
	if err := os.WriteFile(live, []byte("// intentionally divergent bytes\n"), 0o644); err != nil {
		t.Fatalf("corrupt %s: %v", p, err)
	}

	r := checkManagedDrift(root)
	// F6: no-origin existing file → migration-stalled INFO, not drift FAIL.
	if r.tier != tierInfo {
		t.Fatalf("want INFO for no-origin divergent file (F6 migration-stalled), got %s: %s", r.tier, r.detail)
	}
	if !strings.Contains(r.detail, "migration-stalled") {
		t.Errorf("INFO detail should report migration-stalled; got %q", r.detail)
	}
	// With the origin store removed, ALL managed files become migration-stalled
	// (every file has !hadOrigin). The specific corrupted path may be capped out
	// of the path list (>10 entries), so verify the count and the cap note
	// instead of the specific path name.
	if !strings.Contains(r.detail, "more") {
		t.Errorf("INFO detail should show capped path list (170+ stalled); got %q", r.detail)
	}
}

// TestManagedDrift_ConsumerEdit_Preserved_NotDrift is the origin-hash B3
// regression lock: a platform_managed file the consumer EDITED (live diverged
// from the platform's recorded origin hash) is a SANCTIONED state — update
// deliberately preserves it (ActionManagedDiverged) — so doctor must report it
// as a non-failing consumer-preserved signal (tierInfo), NOT as a perpetual
// drifted FAIL whose prescribed remedy (`update`) is a no-op. This keeps doctor
// and update in agreement on day one of the origin-hash feature.
func TestManagedDrift_ConsumerEdit_Preserved_NotDrift(t *testing.T) {
	root := t.TempDir()
	seamInstallInto(t, root)

	// Precondition: the install recorded an origin-hash store.
	if _, err := originhash.Read(root); err != nil {
		t.Fatalf("precondition: origin-hash store should be readable after install: %v", err)
	}

	// Consumer hand-edits a platform_managed AUTHORED (non-regenerated) file
	// (the surface where vh-video-maker's rule 6 lives only in the render).
	// findLiveAuthoredPlatformManagedPath excludes the regenerated set
	// (allowed-commands.js, opencode.jsonc) — a consumer edit to a regenerated
	// path is genuine drift (update overwrites it), not a preserved state, so
	// picking one would silently flip this test's expected INFO verdict to FAIL.
	p := findLiveAuthoredPlatformManagedPath(t, root)
	live := filepath.Join(root, filepath.FromSlash(p))
	const consumerEdit = "// CONSUMER HAND-EDIT (rule 6) — update must preserve, doctor must not FAIL\n"
	if err := os.WriteFile(live, []byte(consumerEdit), 0o644); err != nil {
		t.Fatalf("consumer-edit %s: %v", p, err)
	}

	r := checkManagedDrift(root)
	// Must NOT be a FAIL: the consumer edit is a sanctioned preserved state.
	if r.tier == tierFail {
		t.Fatalf("consumer-edited managed file must NOT drift-FAIL (update preserves it); got FAIL: %s", r.detail)
	}
	// Must surface as non-failing consumer-preserved (tierInfo), naming the path.
	if r.tier != tierInfo {
		t.Fatalf("want INFO (consumer-preserved) for consumer-edited managed file, got %s: %s", r.tier, r.detail)
	}
	if !strings.Contains(r.detail, "consumer-preserved") {
		t.Errorf("INFO detail should mention consumer-preserved; got %q", r.detail)
	}
	if !strings.Contains(r.detail, p) {
		t.Errorf("INFO detail should name the consumer-preserved path %q; got %q", p, r.detail)
	}
}

// TestManagedDrift_RegeneratedConsumerEdit_IsGenuineDrift is the R4-B1
// regression lock: a consumer edit to a platform-REGENERATED managed file
// (allowed-commands.js) is GENUINE drift, NOT a consumer-preserved signal —
// because seamApply EXEMPTS regenerated paths from origin-hash preservation
// (RegeneratedPlatformPaths) and OVERWRITES a consumer edit to keep the file
// byte-sync with the emitted permission blocks. doctor's consumer-preserved
// carve-out must agree with update's overwrite behavior: reporting it as
// "update preserves your edit" would be a false promise. This is the mirror of
// TestManagedDrift_ConsumerEdit_Preserved_NotDrift for the regenerated path.
func TestManagedDrift_RegeneratedConsumerEdit_IsGenuineDrift(t *testing.T) {
	root := t.TempDir()
	seamInstallInto(t, root)

	// Precondition: the install recorded an origin-hash store (incl. for
	// allowed-commands.js, which apply records even though it is exempt from the
	// divergence SKIP — recording is harmless and keeps the store complete).
	if _, err := originhash.Read(root); err != nil {
		t.Fatalf("precondition: origin-hash store should be readable after install: %v", err)
	}

	// Consumer edits the REGENERATED managed file (allowed-commands.js).
	p := ".opencode/repo-configs/allowed-commands.js"
	live := filepath.Join(root, filepath.FromSlash(p))
	if _, err := os.Stat(live); err != nil {
		t.Fatalf("precondition: %s should exist after install: %v", p, err)
	}
	if err := os.WriteFile(live, []byte("// CONSUMER CUSTOMIZATION (regenerated file — update WILL overwrite)\n"), 0o644); err != nil {
		t.Fatalf("consumer-edit %s: %v", p, err)
	}

	r := checkManagedDrift(root)
	// MUST be a FAIL: update overwrites this file (exempt from preservation), so
	// the drift is real and the "run update" remedy is correct. A tierInfo
	// consumer-preserved here would be a false promise (R4-B1 regression).
	if r.tier != tierFail {
		t.Fatalf("consumer-edited REGENERATED managed file (%s) must drift-FAIL (update overwrites it), got %s: %s", p, r.tier, r.detail)
	}
	if !strings.Contains(r.detail, p) {
		t.Errorf("FAIL detail should name the drifted regenerated path %q; got %q", p, r.detail)
	}
	if strings.Contains(r.detail, "consumer-preserved") {
		t.Errorf("REGENERATED managed file must NOT be reported consumer-preserved (update overwrites it); got %q", r.detail)
	}
}

// TestEffectiveRegeneratedPaths_OpencodeJSONCGatedByOriginStore is the F3
// unit-test lock for the F6 coordination hook. opencode.jsonc is DECLARED in
// regeneratedPlatformPaths but EFFECTIVELY regenerated only when a valid origin
// record exists (originStore != nil). Pre-migration (nil store), opencode.jsonc
// is dropped from the effective set so the F6 first-run stall retains authority
// over a colliding pre-feature baseline. allowed-commands.js is ALWAYS
// regenerated (never gated by the origin store).
func TestEffectiveRegeneratedPaths_OpencodeJSONCGatedByOriginStore(t *testing.T) {
	declared := map[string]bool{
		allowedCommandsRel: true,
		opencodeJSONCRel:   true,
	}

	// Pre-migration: no valid origin store → opencode.jsonc excluded (F6 hook).
	pre := effectiveRegeneratedPaths(declared, nil)
	if !pre[allowedCommandsRel] {
		t.Errorf("allowed-commands.js must ALWAYS be effectively regenerated")
	}
	if pre[opencodeJSONCRel] {
		t.Errorf("opencode.jsonc must NOT be effectively regenerated pre-migration (nil store); got included")
	}

	// Partial-migration: store EXISTS but opencode.jsonc has NO entry (it was
	// stalled on the first run). opencode.jsonc is still excluded — the F6
	// stall retains authority until opencode.jsonc's disposition is resolved.
	partial := originhash.New()
	postPartial := effectiveRegeneratedPaths(declared, partial)
	if !postPartial[allowedCommandsRel] {
		t.Errorf("allowed-commands.js must be effectively regenerated (partial-migration)")
	}
	if postPartial[opencodeJSONCRel] {
		t.Errorf("opencode.jsonc must NOT be effectively regenerated when it has no origin entry (partial-migration); got included")
	}

	// Post-migration: store exists AND opencode.jsonc HAS an origin entry →
	// opencode.jsonc included (the migration resolved its disposition).
	resolved := originhash.New()
	resolved.OriginHashes[opencodeJSONCRel] = originhash.Digest([]byte("resolved"))
	postResolved := effectiveRegeneratedPaths(declared, resolved)
	if !postResolved[allowedCommandsRel] {
		t.Errorf("allowed-commands.js must be effectively regenerated post-migration")
	}
	if !postResolved[opencodeJSONCRel] {
		t.Errorf("opencode.jsonc must be effectively regenerated post-migration (origin entry exists); got excluded")
	}
}

// TestManagedDrift_OpencodeJSONC_RegeneratedConsumerEdit_IsGenuineDrift is the
// F3 behavioral lock (mirror of TestManagedDrift_RegeneratedConsumerEdit_IsGenuineDrift
// for opencode.jsonc). Post-migration (valid origin store exists), a consumer
// edit to opencode.jsonc is GENUINE drift — update overwrites it because the
// permission emitter canonically regenerates it on every apply, and doctor's
// consumer-preserved carve-out must NOT promise to preserve an edit update will
// discard. This proves the effective-regenerated admission works end-to-end
// through doctor's drift check.
func TestManagedDrift_OpencodeJSONC_RegeneratedConsumerEdit_IsGenuineDrift(t *testing.T) {
	root := t.TempDir()
	seamInstallInto(t, root)

	// Precondition: the install recorded a valid origin-hash store (non-nil).
	// This is what activates opencode.jsonc's effective-regenerated status.
	store, err := originhash.Read(root)
	if err != nil {
		t.Fatalf("precondition: origin-hash store should be readable after install: %v", err)
	}
	if store == nil {
		t.Fatalf("precondition: origin-hash store should be non-nil after install")
	}

	// Consumer edits opencode.jsonc (the REGENERATED managed file).
	p := opencodeJSONCRel
	live := filepath.Join(root, filepath.FromSlash(p))
	if _, err := os.Stat(live); err != nil {
		t.Fatalf("precondition: %s should exist after install: %v", p, err)
	}
	if err := os.WriteFile(live, []byte("// CONSUMER HAND-EDIT (regenerated file — update WILL overwrite)\n"), 0o644); err != nil {
		t.Fatalf("consumer-edit %s: %v", p, err)
	}

	r := checkManagedDrift(root)
	// MUST be a FAIL: with a valid origin store, opencode.jsonc is effectively
	// regenerated, so a consumer edit is genuine drift (update overwrites it).
	if r.tier != tierFail {
		t.Fatalf("consumer-edited opencode.jsonc must drift-FAIL post-migration (update overwrites it), got %s: %s", r.tier, r.detail)
	}
	if !strings.Contains(r.detail, p) {
		t.Errorf("FAIL detail should name the drifted regenerated path %q; got %q", p, r.detail)
	}
	if strings.Contains(r.detail, "consumer-preserved") {
		t.Errorf("opencode.jsonc must NOT be reported consumer-preserved post-migration; got %q", r.detail)
	}
}

// TestManagedDrift_OverrideProjectOwned_Divergent_Preserved: the core A2 fix. A
// platform_managed path raised to project_owned via harness-ownership.yml, with
// divergent live bytes, must report as a NON-FAILING preserved (tierInfo) signal
// — NOT as a perpetual drifted FAIL. update preserves project_owned divergences
// by design (substrate.Apply ActionProjectPreserved); doctor must agree.
func TestManagedDrift_OverrideProjectOwned_Divergent_Preserved(t *testing.T) {
	root := t.TempDir()
	seamInstallInto(t, root)
	p := findLivePlatformManagedPath(t, root)

	// Diverge the live bytes (this is what would have been drift before A2).
	live := filepath.Join(root, filepath.FromSlash(p))
	if err := os.WriteFile(live, []byte("// hand-curated project content; must be preserved\n"), 0o644); err != nil {
		t.Fatalf("diverge %s: %v", p, err)
	}
	// Raise the path to project_owned via the S2 override authority.
	writeOwnershipOverrides(t, root, map[string]string{p: string(ownership.ClassProjectOwned)})

	r := checkManagedDrift(root)
	if r.tier != tierInfo {
		t.Fatalf("want INFO (preserved) for overridden+divergent project_owned path, got %s: %s", r.tier, r.detail)
	}
	if !strings.Contains(r.detail, "preserved") {
		t.Errorf("INFO detail should mention preserved; got %q", r.detail)
	}
}

// TestManagedDrift_DefaultNonManaged_Divergent_NotPreserved is the F3 regression
// guard. A path whose DEFAULT ownership class is project_owned (a render-
// independent seed like README.md, Makefile, CLAUDE.md, .gitignore,
// docs/planning/backlog.md, docs/planning/roadmap.md, or
// forbidden-patterns.project.js) that diverges from the render must be SILENTLY
// SKIPPED — NOT counted as `preserved` and NOT labeled "project-preserved
// (ownership override)". These diverge by design (operator-curated) and are NOT
// ownership overrides. Only a genuine override-raise (Origin == OriginOverrideRaise)
// may surface as preserved.
//
// Candidate-set note: the helper deliberately restricts to ClassProjectOwned.
// platform_armed (vh-harness-profile.yml) and external_generated (recon data)
// are NOT safe candidates here — the re-render inside checkManagedDrift reads
// them as inputs (readProfileAnswers / recon loader), so corrupting them would
// corrupt the re-render and trip an orthogonal drift failure unrelated to the
// F3 assertion. ClassProjectOwned seeds are never read as render inputs.
//
// This is the exact gap that let the A2 over-broadening (F3) slip: the preserved
// branch fired for ANY effective class != platform_managed, mislabeling the 8
// default-class files in this dogfood repo on every install. The fix narrows the
// gate to origin == override-raise; this test pins the narrowed behavior.
func TestManagedDrift_DefaultNonManaged_Divergent_NotPreserved(t *testing.T) {
	root := t.TempDir()
	seamInstallInto(t, root)
	p := findLiveDefaultNonManagedPath(t, root)
	// Diverge the live bytes — this is the condition that, pre-fix, was mislabeled
	// "project-preserved (ownership override)".
	live := filepath.Join(root, filepath.FromSlash(p))
	if err := os.WriteFile(live, []byte("// intentionally divergent default-class bytes\n"), 0o644); err != nil {
		t.Fatalf("corrupt %s: %v", p, err)
	}
	// NO harness-ownership.yml — this is a default-class divergence, not an override.

	r := checkManagedDrift(root)
	if r.tier != tierPass {
		t.Fatalf("want PASS (default-class divergence is silent, not preserved) for %s (default class on this corpus), got %s: %s",
			p, r.tier, r.detail)
	}
	if strings.Contains(r.detail, "preserved") {
		t.Errorf("PASS detail must not mention preserved for a default-class divergence; got %q", r.detail)
	}
	if !strings.Contains(r.detail, "in sync") {
		t.Errorf("PASS detail should say 'in sync'; got %q", r.detail)
	}
}

// TestManagedDrift_OverrideProjectOwned_MissingFile_NotPreserved: an overridden
// project_owned path that is MISSING from disk must NOT be counted as preserved
// (a different condition) and must NOT be counted as missing/drifted. A raised
// path is the operator's concern — update never seeds or touches it — so its
// absence is silent. This guards against conflating preserved with missing.
func TestManagedDrift_OverrideProjectOwned_MissingFile_NotPreserved(t *testing.T) {
	root := t.TempDir()
	seamInstallInto(t, root)
	p := findLivePlatformManagedPath(t, root)

	// Remove the live file entirely, then raise it to project_owned.
	live := filepath.Join(root, filepath.FromSlash(p))
	if err := os.Remove(live); err != nil {
		t.Fatalf("remove %s: %v", p, err)
	}
	writeOwnershipOverrides(t, root, map[string]string{p: string(ownership.ClassProjectOwned)})

	r := checkManagedDrift(root)
	// No drift, no missing (the path is no longer platform_managed-effective),
	// no preserved divergence (file is absent). Outcome is a clean PASS with no
	// "preserved" mention.
	if r.tier != tierPass {
		t.Fatalf("want PASS (missing project_owned path is silent), got %s: %s", r.tier, r.detail)
	}
	if strings.Contains(r.detail, "preserved") {
		t.Errorf("missing raised path must not be reported as preserved; got %q", r.detail)
	}
	if strings.Contains(r.detail, "missing") {
		t.Errorf("missing raised path must not be reported as missing; got %q", r.detail)
	}
}

// TestManagedDrift_InvalidOverride_FailsClean: a present-but-invalid ownership
// override (unknown class literal) must FAIL cleanly rather than silently
// honoring or ignoring the amendment. Validation happens in one of two layers —
// readOwnershipOverrides rejects unknown class literals early, ownership.Resolve
// rejects downgrades / unknown paths — and doctor surfaces whichever fires so
// the operator fixes the override file.
func TestManagedDrift_InvalidOverride_FailsClean(t *testing.T) {
	root := t.TempDir()
	seamInstallInto(t, root)
	p := findLivePlatformManagedPath(t, root)
	writeOwnershipOverrides(t, root, map[string]string{p: "not-a-real-class"})

	r := checkManagedDrift(root)
	if r.tier != tierFail {
		t.Fatalf("want FAIL for invalid override class, got %s: %s", r.tier, r.detail)
	}
	if !strings.Contains(r.detail, "ownership") {
		t.Errorf("FAIL detail should name the ownership validation error; got %q", r.detail)
	}
	if !strings.Contains(r.detail, "not-a-real-class") {
		t.Errorf("FAIL detail should name the offending class; got %q", r.detail)
	}
}

// TestPreflight_PreservedIsNonBlocking: end-to-end via the preflight entry path.
// An overridden+divergent project_owned path surfaces as INFO (preserved) and
// preflight must treat it as PASS — never blocking install/update on a preserved
// file. Verifies the shared checkManagedDrift fix flows through preflight's
// tier-handling correctly (failed count stays 0 -> exit 0).
func TestPreflight_PreservedIsNonBlocking(t *testing.T) {
	root := t.TempDir()
	seamInstallInto(t, root)
	p := findLivePlatformManagedPath(t, root)

	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(p)),
		[]byte("// hand-curated; ownership-raised\n"), 0o644); err != nil {
		t.Fatalf("diverge %s: %v", p, err)
	}
	writeOwnershipOverrides(t, root, map[string]string{p: string(ownership.ClassProjectOwned)})

	var out string
	var runErr error
	runWithCwd(t, root, func() {
		cmd, buf := newOutCmd()
		runErr = runPreflight(cmd, []string{})
		out = buf.String()
	})

	if runErr != nil {
		t.Fatalf("preflight must PASS (non-blocking) on a preserved file; got err=%v out=%q", runErr, out)
	}
	if !strings.Contains(out, "result: PASS") {
		t.Fatalf("preflight output should report PASS; got:\n%s", out)
	}
	// The managed-drift row should carry the INFO tier + preserved detail, proving
	// the signal is surfaced (not silently swallowed) while still non-blocking.
	if !strings.Contains(out, "INFO") || !strings.Contains(out, "preserved") {
		t.Errorf("preflight should surface the INFO/preserved managed-drift row; got:\n%s", out)
	}
}

// TestManagedDrift_Divergent_NamesPathAndBothRemedies is the surface-at-friction
// regression lock for the managed-drift FAIL detail (doctor + the seam preflight
// path, which both call checkManagedDrift). The FAIL message MUST name the
// drifted path and surface BOTH remedies — the destructive `update` AND the
// non-destructive overlay-pack promotion — so an operator can route without
// losing a deliberate edit. Pins researches/decisions/2026-08-04-capability-
// discovery-audit.md §6 entry 1 (the SIGNED OFF entry).
//
// Post-F6: genuine drift requires origin == live (stale-but-unedited) but
// live != staged. We achieve this by corrupting the live file and then fixing
// the origin entry to match the corrupted content (simulating "the platform
// previously wrote these bytes"). Removing the origin store now produces
// migration-stalled INFO (F6), not drift FAIL.
func TestManagedDrift_Divergent_NamesPathAndBothRemedies(t *testing.T) {
	root := t.TempDir()
	seamInstallInto(t, root)
	p := findLivePlatformManagedPath(t, root)
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(p)),
		[]byte("// intentionally divergent bytes\n"), 0o644); err != nil {
		t.Fatalf("corrupt %s: %v", p, err)
	}
	// Fix origin to match the corrupted live so the divergence is genuine drift
	// (stale-but-unedited: origin==live, live!=staged), NOT F6 migration-stalled.
	fixOriginToLive(t, root, p)
	r := checkManagedDrift(root)
	if r.tier != tierFail {
		t.Fatalf("want FAIL, got %s: %s", r.tier, r.detail)
	}
	// Names the drifted path (self-routing — the load-bearing fix).
	if !strings.Contains(r.detail, p) {
		t.Errorf("FAIL detail should name the drifted path %q; got %q", p, r.detail)
	}
	// Destructive remedy.
	if !strings.Contains(r.detail, "vh-agent-harness update") {
		t.Errorf("FAIL detail should name the destructive `update` remedy; got %q", r.detail)
	}
	// Non-destructive overlay-promotion remedy.
	if !strings.Contains(r.detail, "overlay pack source") || !strings.Contains(r.detail, ".vh-agent-harness/overlays/<pack>/") {
		t.Errorf("FAIL detail should name the non-destructive overlay-pack remedy; got %q", r.detail)
	}
}

// TestCapPathList exercises the capped path listing used in managed-drift FAIL
// details: empty → "", under-cap → sorted, over-cap → truncation note pointing
// at `vh-agent-harness diff`.
func TestCapPathList(t *testing.T) {
	if got := capPathList("drifted", nil); got != "" {
		t.Errorf("empty paths → want \"\", got %q", got)
	}
	got := capPathList("drifted", []string{".opencode/b.md", ".opencode/a.md"})
	if want := "drifted: .opencode/a.md, .opencode/b.md"; got != want {
		t.Errorf("under-cap should sort + join; want %q, got %q", want, got)
	}
	// Over-cap: truncate + note. driftDetailPathCap is 10.
	paths := make([]string, driftDetailPathCap+3)
	for i := range paths {
		paths[i] = fmt.Sprintf(".opencode/f%02d.md", i)
	}
	got = capPathList("drifted", paths)
	if !strings.Contains(got, "and 3 more") {
		t.Errorf("over-cap should carry truncation note; got %q", got)
	}
	if !strings.Contains(got, "vh-agent-harness diff") {
		t.Errorf("over-cap note should point at `vh-agent-harness diff`; got %q", got)
	}
}

// TestFormatManagedDriftFail exercises the shared FAIL-detail formatter used by
// both checkManagedDrift (doctor + preflight seam) and checkDrift (preflight
// legacy): it appends BOTH remedies regardless of which category triggered,
// and omits empty category segments.
func TestFormatManagedDriftFail(t *testing.T) {
	got := formatManagedDriftFail("2 drifted, 0 missing of 50 managed",
		[]string{".opencode/a.md", ".opencode/b.md"}, nil)
	// Counts header leads.
	if !strings.HasPrefix(got, "2 drifted, 0 missing of 50 managed") {
		t.Errorf("summary header should lead; got %q", got)
	}
	// Drifted paths named.
	if !strings.Contains(got, "drifted: .opencode/a.md, .opencode/b.md") {
		t.Errorf("drifted paths should be named; got %q", got)
	}
	// Missing segment omitted when empty.
	if strings.Contains(got, "missing:") {
		t.Errorf("empty missing set should not emit a missing segment; got %q", got)
	}
	// Both remedies.
	if !strings.Contains(got, "vh-agent-harness update") || !strings.Contains(got, "DESTRUCTIVE") {
		t.Errorf("destructive remedy must appear; got %q", got)
	}
	if !strings.Contains(got, "overlay pack source") {
		t.Errorf("non-destructive remedy must appear; got %q", got)
	}
}

// TestManagedDrift_ConsumerDelete_Preserved_NotMissing is the F1 regression
// lock: a platform_managed file the consumer DELETED (but the platform
// previously rendered, so it has a recorded origin hash) must be reported as a
// NON-FAILING consumer-preserved-deletion signal (tierInfo), mirroring Apply —
// which routes this exact case to ActionManagedDiverged "consumer-deleted; not
// re-seeded" and does NOT restore the file. It must NOT be counted as missing
// drift, because the missing-drift FAIL detail carries the remedy "run update to
// re-render drifted/missing files from the corpus" — a FALSE claim for a path
// update will NOT re-seed. This keeps doctor and Apply in agreement on day one
// of the deletion-reconciliation (F1), the mirror of
// TestManagedDrift_ConsumerEdit_Preserved_NotDrift for the deletion half.
func TestManagedDrift_ConsumerDelete_Preserved_NotMissing(t *testing.T) {
	root := t.TempDir()
	seamInstallInto(t, root)

	// Precondition: the install recorded an origin-hash store (so the deletion
	// is TRACKED — the platform previously rendered this path).
	if store, err := originhash.Read(root); err != nil || store == nil {
		t.Fatalf("precondition: origin-hash store should be readable+non-nil after install: %v", err)
	}

	// Consumer deletes an AUTHORED (non-regenerated) managed file.
	p := findLiveAuthoredPlatformManagedPath(t, root)
	live := filepath.Join(root, filepath.FromSlash(p))
	if err := os.Remove(live); err != nil {
		t.Fatalf("consumer-delete %s: %v", p, err)
	}

	r := checkManagedDrift(root)
	// Must NOT FAIL: the consumer deletion is a sanctioned preserved state.
	if r.tier == tierFail {
		t.Fatalf("consumer-deleted managed file must NOT missing-FAIL (update respects the deletion); got FAIL: %s", r.detail)
	}
	// Must surface as non-failing consumer-preserved-deletion (tierInfo), naming the path.
	if r.tier != tierInfo {
		t.Fatalf("want INFO (consumer-preserved-deletion) for consumer-deleted managed file, got %s: %s", r.tier, r.detail)
	}
	if !strings.Contains(r.detail, "consumer-preserved") {
		t.Errorf("INFO detail should mention consumer-preserved; got %q", r.detail)
	}
	if !strings.Contains(r.detail, "deletion") {
		t.Errorf("INFO detail should distinguish deletion from edit; got %q", r.detail)
	}
	if !strings.Contains(r.detail, p) {
		t.Errorf("INFO detail should name the consumer-preserved-deletion path %q; got %q", p, r.detail)
	}
	// The deletion signal must NOT carry the false "run update to re-render
	// missing files" claim (update will NOT restore a deletion it respects).
	if strings.Contains(r.detail, "re-render drifted/missing") {
		t.Errorf("INFO detail must NOT prescribe re-rendering missing files for a deletion update respects; got %q", r.detail)
	}
}

// TestManagedDrift_UntrackedDelete_IsGenuineMissing is the F1 counterweight: a
// managed file with NO recorded origin (bootstrap / pre-feature) that is absent
// from disk is GENUINE missing drift — update WILL seed it. doctor must FAIL it
// (the missing-drift remedy "run update" is accurate here), NOT carve it out as
// consumer-preserved. This proves the F1 carve-out is gated on a TRACKED origin
// (mirroring Apply's bootstrap handling) and does not swallow genuine missing.
func TestManagedDrift_UntrackedDelete_IsGenuineMissing(t *testing.T) {
	root := t.TempDir()
	seamInstallInto(t, root)
	// Remove the origin-hash store so the deleted file has NO recorded origin
	// (bootstrap / pre-feature semantics): genuine missing, not a tracked
	// consumer deletion.
	if err := os.Remove(originhash.FilePath(root)); err != nil && !os.IsNotExist(err) {
		t.Fatalf("remove origin-hash store: %v", err)
	}
	p := findLiveAuthoredPlatformManagedPath(t, root)
	live := filepath.Join(root, filepath.FromSlash(p))
	if err := os.Remove(live); err != nil {
		t.Fatalf("consumer-delete %s: %v", p, err)
	}

	r := checkManagedDrift(root)
	if r.tier != tierFail {
		t.Fatalf("want FAIL for untracked (no origin) missing managed file, got %s: %s", r.tier, r.detail)
	}
	if !strings.Contains(r.detail, "missing") {
		t.Errorf("FAIL detail should report missing; got %q", r.detail)
	}
	if !strings.Contains(r.detail, p) {
		t.Errorf("FAIL detail should name the missing path %q; got %q", p, r.detail)
	}
	if strings.Contains(r.detail, "consumer-preserved") {
		t.Errorf("untracked missing must NOT be carved out as consumer-preserved; got %q", r.detail)
	}
}

// TestManagedDrift_RegeneratedDelete_IsGenuineMissing is the F1 mirror of
// TestManagedDrift_RegeneratedConsumerEdit_IsGenuineDrift for the deletion
// case: a deleted platform-REGENERATED managed file (allowed-commands.js) is
// GENUINE missing — seamApply EXEMPTS regenerated paths from origin-hash
// preservation and would RE-SEED/re-emit it on the next update. doctor's
// consumer-preserved-deletion carve-out must agree: reporting it as "update
// respects your deletion" would be a false promise. A regenerated deletion
// must FAIL as genuine missing.
func TestManagedDrift_RegeneratedDelete_IsGenuineMissing(t *testing.T) {
	root := t.TempDir()
	seamInstallInto(t, root)

	if store, err := originhash.Read(root); err != nil || store == nil {
		t.Fatalf("precondition: origin-hash store should be readable after install: %v", err)
	}

	// Delete the REGENERATED managed file (allowed-commands.js). It HAS a
	// recorded origin (apply records regenerated paths too), but the
	// regenerated exemption must keep it genuine-missing.
	p := ".opencode/repo-configs/allowed-commands.js"
	live := filepath.Join(root, filepath.FromSlash(p))
	if _, err := os.Stat(live); err != nil {
		t.Fatalf("precondition: %s should exist after install: %v", p, err)
	}
	if err := os.Remove(live); err != nil {
		t.Fatalf("delete %s: %v", p, err)
	}

	r := checkManagedDrift(root)
	if r.tier != tierFail {
		t.Fatalf("deleted REGENERATED managed file (%s) must missing-FAIL (update re-emits it), got %s: %s", p, r.tier, r.detail)
	}
	if !strings.Contains(r.detail, "missing") {
		t.Errorf("FAIL detail should report missing; got %q", r.detail)
	}
	if strings.Contains(r.detail, "consumer-preserved") {
		t.Errorf("deleted REGENERATED managed file must NOT be reported consumer-preserved (update re-emits it); got %q", r.detail)
	}
}

// TestManagedDrift_CorruptOriginState_DeleteFailsLoud locks the fail-LOUD
// contract for the F1 carve-out when the origin-hash store itself is corrupt or
// unreadable: doctor tolerates the read error (it is diagnostic) by falling
// back to a nil store, which makes Lookup report no prior origin everywhere.
// A consumer-deleted file under a corrupt store is therefore NOT carved out as
// consumer-preserved (no tracked origin to honor) — it FAILs as genuine
// missing, which is the correct fail-LOUD behavior (the hard fail-closed
// contract lives in Apply, not doctor). doctor must not crash and must not
// falsely preserve the deletion.
func TestManagedDrift_CorruptOriginState_DeleteFailsLoud(t *testing.T) {
	root := t.TempDir()
	seamInstallInto(t, root)

	// Plant a corrupt origin-hash store (malformed JSON).
	dir := filepath.Join(root, originhash.DirName)
	if err := os.MkdirAll(dir, 0o75); err != nil {
		t.Fatalf("mkdir store dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, originhash.FileName),
		[]byte("{ this is not valid json "), 0o644); err != nil {
		t.Fatalf("plant corrupt store: %v", err)
	}

	// Consumer deletes an authored managed file. Under a healthy store this
	// would be consumer-preserved-deletion; under a corrupt store it MUST be
	// genuine missing (fail-LOUD), never falsely preserved.
	p := findLiveAuthoredPlatformManagedPath(t, root)
	live := filepath.Join(root, filepath.FromSlash(p))
	if err := os.Remove(live); err != nil {
		t.Fatalf("consumer-delete %s: %v", p, err)
	}

	r := checkManagedDrift(root)
	// doctor did not crash (it returned a result) and FAILs loud as missing.
	if r.tier != tierFail {
		t.Fatalf("want FAIL (genuine missing) under corrupt origin store, got %s: %s", r.tier, r.detail)
	}
	if !strings.Contains(r.detail, "missing") {
		t.Errorf("FAIL detail should report missing; got %q", r.detail)
	}
	if strings.Contains(r.detail, "consumer-preserved") {
		t.Errorf("corrupt origin store must NOT allow a deletion to be falsely preserved; got %q", r.detail)
	}
}
