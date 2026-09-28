package cli

import (
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/vhqtvn/vh-agent-harness/internal/lineage"
	"github.com/vhqtvn/vh-agent-harness/internal/originhash"
	"github.com/vhqtvn/vh-agent-harness/internal/overlay"
	"github.com/vhqtvn/vh-agent-harness/internal/renderstate"
	"github.com/vhqtvn/vh-agent-harness/internal/runshape"
)

// ── greenfield bootstrap: behavior contract ──────────────────────────────────
//
// These tests pin the bootstrap rework's load-bearing behaviors:
//
//   1. bare greenfield install seeds the FROZEN FULL recipe and renders the
//      complete shipped surface (all capabilities + all overlays, together);
//   2. --full is byte-identical to the bare default;
//   3. --minimal seeds the historical minimal default;
//   4. --profile <preset> seeds the default carrying that preset;
//   5. selectors are rejected on an existing live profile BEFORE any write;
//   5b. an existing but UNREADABLE live profile fails closed on EVERY path
//      (selector and bare default alike) — it must never be mistaken for
//      greenfield and seeded over;
//   6. conflicting/unknown selectors are usage errors;
//   7. preview (--dry-run) and apply resolve from ONE effective profile and
//      agree on the plan;
//   8. existing installs are NEVER backfilled: a minimal install stays minimal
//      through bare re-install and update (the sacred existing-install
//      invariant).

// installIntoWithSelector runs runInstall with the given selector fields set
// (exactly one of minimal/full/profilePreset; all empty = bare default).
func installIntoWithSelector(t *testing.T, root string, full, minimal bool, profilePreset string) (string, error) {
	t.Helper()
	installFl = newInstallFlags()
	installFl.target = root
	installFl.full = full
	installFl.minimal = minimal
	installFl.profile = profilePreset
	cmd, buf := newOutCmd()
	err := runInstall(cmd, []string{})
	return buf.String(), err
}

// assertLivePathsExist asserts every rel path exists under root.
func assertLivePathsExist(t *testing.T, root string, rels []string, msg string) {
	t.Helper()
	for _, rel := range rels {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if _, err := os.Stat(p); os.IsNotExist(err) {
			t.Errorf("%s: expected %s to exist", msg, rel)
		}
	}
}

// packUnitLivePaths enumerates the LIVE .opencode-relative paths an embedded
// pack renders (by rendering it into a scratch staging dir), so surface
// assertions stay in sync with pack contents automatically.
func packUnitLivePaths(t *testing.T, packName string) []string {
	t.Helper()
	pack, err := overlay.OpenPack(packName)
	if err != nil {
		t.Fatalf("open embedded pack %s: %v", packName, err)
	}
	staging := t.TempDir()
	rendered, err := pack.RenderUnits(staging, map[string]string{})
	if err != nil {
		t.Fatalf("render pack %s units: %v", packName, err)
	}
	return rendered
}

// TestInstall_GreenfieldDefaultSeedsFrozenFullRecipe is the bootstrap crux at
// the CLI level: a BARE no-flag install into a greenfield target seeds the
// frozen full recipe at the live profile path and renders the complete shipped
// surface — every ledger `in` capability's gated outputs AND every ledger `in`
// overlay pack's units, together, with the seeded profile bytes exactly equal
// to the embedded recipe.
func TestInstall_GreenfieldDefaultSeedsFrozenFullRecipe(t *testing.T) {
	root := t.TempDir()
	out, err := installIntoWithSelector(t, root, false, false, "")
	if err != nil {
		t.Fatalf("bare greenfield install: %v (out=%q)", err, out)
	}

	// The seeded live profile IS the frozen recipe, byte-for-byte.
	recipe, rerr := bootstrapRecipeBytes()
	if rerr != nil {
		t.Fatalf("load frozen recipe: %v", rerr)
	}
	live, perr := os.ReadFile(filepath.Join(root, harnessProfileName))
	if perr != nil {
		t.Fatalf("read seeded profile: %v", perr)
	}
	if string(live) != string(recipe) {
		t.Errorf("seeded profile must equal the frozen full recipe bytes;\n want (%d bytes) %q\n got  (%d bytes) %q",
			len(recipe), recipe, len(live), live)
	}

	// Every `in` overlay pack's units rendered.
	for _, e := range bootstrapOverlayLedger {
		if e.decision != "in" {
			continue
		}
		assertLivePathsExist(t, root, packUnitLivePaths(t, e.name), "full install pack "+e.name)
	}
	// Every `in` capability's gated core outputs rendered (media-perception and
	// worker-read-only own CoreOutputs; gated-commit/debate/release own agents).
	assertLivePathsExist(t, root, []string{
		".opencode/agents/media-perception.md",
		".opencode/skills/media-perception/SKILL.md",
		".opencode/agents/worker-read-only.md",
		".opencode/agents/committer.md",
		".opencode/agents/debate.md",
		".opencode/agents/releaser.md",
	}, "full install capability outputs")

	// The run reported its effective selection and a replay command.
	if !strings.Contains(out, "selection (source:") {
		t.Errorf("install output must report the effective selection; got:\n%s", out)
	}
	if !strings.Contains(out, "replay: vh-agent-harness install") {
		t.Errorf("install output must print a replay command; got:\n%s", out)
	}
	// A default-full replay is pinned to --full for stability.
	if !strings.Contains(out, "--full") {
		t.Errorf("default-full replay must pin --full explicitly; got:\n%s", out)
	}

	// Doctor is HEALTHY: install's render agrees with doctor's re-render from
	// the seeded profile (preview/apply/re-render parity).
	if dout := seamDoctorOut(t, root); !strings.Contains(dout, "result: HEALTHY") {
		t.Errorf("doctor must be HEALTHY after a full install; got:\n%s", dout)
	}

	// A subsequent update preserves the SELECTION exactly (the armed reconcile
	// may re-marshal the file — comments/serialization — but the recipe's
	// preset, capabilities, and overlays must survive untouched and un-augmented;
	// bootstrap selection never leaks into recurring reconciliation).
	if _, err := seamUpdateOut(t, root); err != nil {
		t.Fatalf("update after full install: %v", err)
	}
	after, err := os.ReadFile(filepath.Join(root, harnessProfileName))
	if err != nil {
		t.Fatalf("re-read profile after update: %v", err)
	}
	for _, want := range []string{
		"profile: supervised",
		"- core/debate", "- core/gated-commit", "- core/media-perception", "- core/release", "- core/worker-read-only",
		"- auto-classifier-pilot", "- contract-invariant-audit-pilot", "- formal-verification-pilot",
		"- frontend-ui-pilot", "- release", "- repo-mail", "- resolve-first-pilot",
	} {
		if !strings.Contains(string(after), want) {
			t.Errorf("update lost recipe selection entry %q; profile after update:\n%s", want, after)
		}
	}
}

// TestInstall_FullFlagMatchesBareDefault proves --full and the bare greenfield
// default are the SAME selection: both targets end with byte-identical live
// profiles.
func TestInstall_FullFlagMatchesBareDefault(t *testing.T) {
	bareRoot, fullRoot := t.TempDir(), t.TempDir()
	if _, err := installIntoWithSelector(t, bareRoot, false, false, ""); err != nil {
		t.Fatalf("bare install: %v", err)
	}
	if _, err := installIntoWithSelector(t, fullRoot, true, false, ""); err != nil {
		t.Fatalf("--full install: %v", err)
	}
	bare, err := os.ReadFile(filepath.Join(bareRoot, harnessProfileName))
	if err != nil {
		t.Fatalf("read bare profile: %v", err)
	}
	full, err := os.ReadFile(filepath.Join(fullRoot, harnessProfileName))
	if err != nil {
		t.Fatalf("read full profile: %v", err)
	}
	if string(bare) != string(full) {
		t.Errorf("--full and the bare default must seed identical bytes;\n bare=%q\n full=%q", bare, full)
	}
}

// TestInstall_MinimalSeedsHistoricalDefault proves --minimal preserves the
// historical shape: the seeded profile is the embedded core default and NO
// capability cluster / release surface renders.
func TestInstall_MinimalSeedsHistoricalDefault(t *testing.T) {
	root := t.TempDir()
	if _, err := installIntoWithSelector(t, root, false, true, ""); err != nil {
		t.Fatalf("--minimal install: %v", err)
	}
	def, err := corpusDefaultProfileBytes()
	if err != nil {
		t.Fatalf("load embedded default: %v", err)
	}
	live, err := os.ReadFile(filepath.Join(root, harnessProfileName))
	if err != nil {
		t.Fatalf("read seeded profile: %v", err)
	}
	if string(live) != string(def) {
		t.Errorf("--minimal must seed the embedded core default bytes;\n want %q\n got %q", def, live)
	}
	// NOTE: gated-commit/debate agent FILES are unconditional core corpus
	// (their gating is the opencode.jsonc roster + permissions, not file
	// existence). The minimal absent-set is therefore the CoreOutputs-gated
	// capability files and the pack-rendered units.
	for _, absent := range []string{
		".opencode/agents/releaser.md",         // release pack unit (not selected)
		".opencode/agents/media-perception.md", // CoreOutputs-gated, unselected
		".opencode/agents/worker-read-only.md", // CoreOutputs-gated, unselected
	} {
		if p := filepath.Join(root, filepath.FromSlash(absent)); pathExists(t, p) {
			t.Errorf("--minimal must not render %s", absent)
		}
	}
}

// TestInstall_ProfilePresetSeedsPreset proves --profile <preset> seeds the
// embedded default carrying that preset: supervised renders the gated-commit +
// debate clusters while release stays dormant (the historical preset meaning;
// supervised is NOT full).
func TestInstall_ProfilePresetSeedsPreset(t *testing.T) {
	root := t.TempDir()
	out, err := installIntoWithSelector(t, root, false, false, "supervised")
	if err != nil {
		t.Fatalf("--profile supervised install: %v (out=%q)", err, out)
	}
	live, err := os.ReadFile(filepath.Join(root, harnessProfileName))
	if err != nil {
		t.Fatalf("read seeded profile: %v", err)
	}
	if !strings.Contains(string(live), "profile: supervised") {
		t.Errorf("--profile supervised must seed `profile: supervised`; got %q", live)
	}
	assertLivePathsExist(t, root, []string{
		".opencode/agents/committer.md",
		".opencode/agents/debate.md",
	}, "supervised preset clusters")
	if p := filepath.Join(root, ".opencode", "agents", "releaser.md"); pathExists(t, p) {
		t.Errorf("supervised preset must keep release dormant (no releaser.md)")
	}
}

// TestInstall_SelectorsRejectedOnExistingProfile proves the sacred invariant's
// front gate: EVERY selector (--full, --minimal, --profile) against a target
// with an existing live profile is rejected as a usage error BEFORE any write —
// the live profile and lineage bytes are untouched by the refused run.
func TestInstall_SelectorsRejectedOnExistingProfile(t *testing.T) {
	root := t.TempDir()
	if _, err := installIntoWithSelector(t, root, false, true, ""); err != nil {
		t.Fatalf("initial --minimal install: %v", err)
	}
	profilePath := filepath.Join(root, harnessProfileName)
	profileBefore, _ := os.ReadFile(profilePath)
	lineageBefore, _ := os.ReadFile(lineage.FilePath(root))

	for name, setter := range map[string]func(){
		"--full":    func() { installFl.full = true },
		"--minimal": func() { installFl.minimal = true },
		"--profile": func() { installFl.profile = "supervised" },
	} {
		installFl = newInstallFlags()
		installFl.target = root
		setter()
		cmd, buf := newOutCmd()
		err := runInstall(cmd, []string{})
		if err == nil {
			t.Errorf("selector %s must be REJECTED on an existing live profile; got success (out=%q)", name, buf.String())
		}
		if !strings.Contains(err.Error(), "refused") {
			t.Errorf("selector %s rejection must explain the refusal; got: %v", name, err)
		}
		profileAfter, _ := os.ReadFile(profilePath)
		if string(profileAfter) != string(profileBefore) {
			t.Errorf("selector %s rejection must not touch the live profile", name)
		}
		lineageAfter, _ := os.ReadFile(lineage.FilePath(root))
		if string(lineageAfter) != string(lineageBefore) {
			t.Errorf("selector %s rejection must not touch lineage", name)
		}
	}
}

// TestInstall_UnreadableLiveProfileFailsClosedAllPaths pins the commit-review
// tier1_b/F1 regression: when the live profile path EXISTS but cannot be read
// (simulated deterministically by a directory where the file belongs — EISDIR,
// uid-independent, unlike a chmod trick that root would defeat), EVERY install
// path — bare default AND every selector — must fail closed with the
// cannot-read error BEFORE any write, never treating the target as greenfield
// and seeding the recipe over whatever is there.
func TestInstall_UnreadableLiveProfileFailsClosedAllPaths(t *testing.T) {
	for name, setter := range map[string]func(){
		"bare default": func() {},
		"--full":       func() { installFl.full = true },
		"--minimal":    func() { installFl.minimal = true },
		"--profile":    func() { installFl.profile = "supervised" },
	} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			// Plant an unreadable "live profile": a directory at the profile
			// path makes os.ReadFile fail with EISDIR (not IsNotExist).
			blocker := filepath.Join(root, harnessProfileName)
			if err := os.MkdirAll(blocker, 0o755); err != nil {
				t.Fatalf("plant unreadable profile blocker: %v", err)
			}

			installFl = newInstallFlags()
			installFl.target = root
			setter()
			cmd, buf := newOutCmd()
			err := runInstall(cmd, []string{})
			if err == nil {
				t.Fatalf("%s must fail closed on an unreadable live profile; got success (out=%q)", name, buf.String())
			}
			if !strings.Contains(err.Error(), "cannot read live profile") {
				t.Errorf("%s: error must name the unreadable live profile; got: %v", name, err)
			}
			// Fail-closed means fail BEFORE any write: the blocker is intact
			// and no lineage was created.
			if fi, statErr := os.Stat(blocker); statErr != nil || !fi.IsDir() {
				t.Errorf("%s: refusal must not touch the live profile path; stat err=%v isDir=%v", name, statErr, fi.IsDir())
			}
			if _, statErr := os.Stat(lineage.FilePath(root)); !os.IsNotExist(statErr) {
				t.Errorf("%s: refusal must not write lineage; stat err=%v", name, statErr)
			}
		})
	}
}

// TestInstall_ConflictingSelectorsRejected proves the selectors are mutually
// exclusive usage errors on a greenfield target.
func TestInstall_ConflictingSelectorsRejected(t *testing.T) {
	cases := []struct {
		name          string
		full, minimal bool
		profile       string
		want          string
	}{
		{name: "full+minimal", full: true, minimal: true, want: "mutually exclusive"},
		{name: "full+profile", full: true, profile: "supervised", want: "mutually exclusive"},
		{name: "minimal+profile", minimal: true, profile: "minimal", want: "mutually exclusive"},
		{name: "unknown preset", profile: "bogus", want: "unknown --profile preset"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			out, err := installIntoWithSelector(t, root, tc.full, tc.minimal, tc.profile)
			if err == nil {
				t.Fatalf("must be rejected; got success (out=%q)", out)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error must mention %q; got: %v", tc.want, err)
			}
			// Rejected before any write: nothing landed in the target.
			if pathExists(t, filepath.Join(root, harnessProfileName)) {
				t.Errorf("rejection must happen BEFORE any write; profile was seeded")
			}
			if pathExists(t, lineage.FilePath(root)) {
				t.Errorf("rejection must happen BEFORE any write; lineage was written")
			}
		})
	}
}

// TestInstall_PreviewApplyParity proves --dry-run and apply resolve from ONE
// effective profile and agree at three levels on two identical greenfield
// targets:
//
//  1. the preview writes NOTHING (pure plan; target stays empty);
//  2. the preview's operator-visible PLAN — the path→action set parsed from
//     the "Would SEED/RECONCILE/OVERWRITE" sections — equals the file set the
//     live apply lands, modulo exactly the post-apply side effects the dry
//     run skips by contract (lineage, run-shape seed, origin-hashes sidecar,
//     rendered-outputs manifest, materialized context docs, empty agent-model
//     seeds; the AGENTS.md compose output needs no modulo entry — root
//     AGENTS.md is a plan-listed rendered file, and compose is a no-op on a
//     greenfield target with no AGENTS.mission.md). Same paths, same write
//     actions: nothing planned-but-missing, nothing applied-but-unplanned.
//     On a greenfield target every planned action must be a WRITE (seed,
//     armed reconcile, or overwrite-of-absent) — a preserve/conflict action
//     would contradict the empty target;
//  3. the applied tree is a FIXPOINT of the plan: a second --dry-run over the
//     applied target reports zero Would-OVERWRITE, zero consumer-edited
//     managed PRESERVEs, and zero conflicts. Overwrite catches bit-rot on
//     origin-clean files; PRESERVE (consumer-edited managed) catches drift on
//     authored files (they route there, not to overwrite, once they diverge
//     from their just-recorded origin hash); on a tree the same apply just
//     wrote, BOTH must be empty — so the applied BYTES equal the planned
//     bytes for every managed path (level 2 proves path parity; this level
//     proves byte parity — a hand-corrupted managed file keeps the path set
//     identical but reappears here).
//
// The applied profile equals the frozen recipe bytes end-to-end.
func TestInstall_PreviewApplyParity(t *testing.T) {
	previewRoot, applyRoot := t.TempDir(), t.TempDir()

	// ── preview: plan only, nothing written ──────────────────────────────
	installFl = newInstallFlags()
	installFl.target = previewRoot
	installFl.dryRun = true
	cmd, buf := newOutCmd()
	if err := runInstall(cmd, []string{}); err != nil {
		t.Fatalf("dry-run install: %v (out=%q)", err, buf.String())
	}
	previewOut := buf.String()
	// Preview reported the same planned effective selection as apply would.
	if !strings.Contains(previewOut, "selection (source: greenfield default") {
		t.Errorf("dry-run must report the planned effective selection; got:\n%s", previewOut)
	}
	// NOTHING was written by the preview.
	for _, absent := range []string{harnessProfileName, lineage.FilePath(previewRoot), ".opencode"} {
		if pathExists(t, filepath.Join(previewRoot, filepath.FromSlash(absent))) {
			t.Errorf("dry-run must not write %s", absent)
		}
	}
	prev := walkTreePaths(t, previewRoot)
	if len(prev) != 0 {
		t.Errorf("preview target must stay empty; found %v", prev)
	}

	// ── apply on an identical greenfield target ─────────────────────────
	if _, err := installIntoWithSelector(t, applyRoot, false, false, ""); err != nil {
		t.Fatalf("apply install: %v", err)
	}

	// ── level 2: plan path/action set == applied file set ───────────────
	plan := parseDryRunPlanSections(t, previewOut)
	if len(plan) == 0 {
		t.Fatalf("preview plan parsed to an empty action set; output:\n%s", previewOut)
	}
	for p, action := range plan {
		if !strings.Contains(action, "SEED") &&
			!strings.Contains(action, "RECONCILE") &&
			!strings.Contains(action, "OVERWRITE") {
			t.Errorf("greenfield plan action for %s must be a write (SEED/RECONCILE/OVERWRITE); got %q", p, action)
		}
	}
	applied := walkTreePaths(t, applyRoot)
	if len(applied) == 0 {
		t.Fatal("apply target must have the rendered tree")
	}
	appliedSet := make(map[string]bool, len(applied))
	for _, p := range applied {
		appliedSet[p] = true
	}
	// sideEffects enumerates the post-apply writes the dry-run plan skips BY
	// CONTRACT (the plan header itself names lineage, run-shape seed, and
	// AGENTS.md compose; the rest are binary-owned records). Derived from the
	// same path helpers the live apply writes through, so this set tracks the
	// code: adding a new post-apply side-effect file MUST update it here, and
	// a silent extra write fails this test loudly instead of passing.
	rel := func(abs string) string {
		t.Helper()
		r, err := filepath.Rel(applyRoot, abs)
		if err != nil {
			t.Fatalf("rel %s: %v", abs, err)
		}
		return filepath.ToSlash(r)
	}
	sideEffects := map[string]bool{
		rel(lineage.FilePath(applyRoot)):               true,
		rel(originhash.FilePath(applyRoot)):            true,
		rel(renderstate.FilePath(applyRoot)):           true,
		path.Join(runshape.DirName, runshape.FileName): true,
	}
	for _, key := range contextDocKeys {
		sideEffects[path.Join(runshape.DirName, contextDocsSubdir, key+".md")] = true
	}
	// Every planned path landed as a file in the applied tree.
	for p, action := range plan {
		if !appliedSet[p] {
			t.Errorf("planned path %s (%s) is MISSING from the applied tree", p, action)
		}
	}
	// Every applied file is either planned or a known post-apply side effect.
	for _, p := range applied {
		if _, ok := plan[p]; ok {
			continue
		}
		if sideEffects[p] {
			continue
		}
		// Empty .local/config/agent-model/<agent> seeds for every {file:} ref
		// in the rendered opencode.jsonc (operator-managed, gitignored).
		if strings.HasPrefix(p, ".local/config/agent-model/") {
			continue
		}
		t.Errorf("applied tree contains %s, which the preview plan does not list and which is not a known post-apply side effect", p)
	}

	// The applied profile equals the frozen recipe (the same bytes the preview
	// planned to seed — asserted by the source line above and here end-to-end).
	recipe, err := bootstrapRecipeBytes()
	if err != nil {
		t.Fatalf("recipe: %v", err)
	}
	live, err := os.ReadFile(filepath.Join(applyRoot, harnessProfileName))
	if err != nil {
		t.Fatalf("read applied profile: %v", err)
	}
	if string(live) != string(recipe) {
		t.Errorf("applied profile must equal the frozen recipe bytes")
	}

	// ── level 3: the applied tree is a fixpoint of the plan ─────────────
	installFl = newInstallFlags()
	installFl.target = applyRoot
	installFl.dryRun = true
	cmd2, buf2 := newOutCmd()
	if err := runInstall(cmd2, []string{}); err != nil {
		t.Fatalf("second dry-run over the applied tree: %v (out=%q)", err, buf2.String())
	}
	secondOut := buf2.String()
	if !strings.Contains(secondOut, "selection (source: existing live profile (preserved)") {
		t.Errorf("second dry-run must resolve from the preserved live profile (one profile authority); got:\n%s", secondOut)
	}
	if strings.Contains(secondOut, "Would OVERWRITE") {
		t.Errorf("applied tree must be a FIXPOINT of the plan — no managed file should reappear as Would OVERWRITE; got:\n%s", secondOut)
	}
	// Authored managed files that drift post-apply route to PRESERVE
	// (consumer-edited managed), not OVERWRITE (origin-hash ownership
	// transfer) — on a tree the SAME apply just wrote, no file can honestly
	// claim consumer edits, so this section must be empty too.
	if strings.Contains(secondOut, "Would PRESERVE (consumer-edited managed)") {
		t.Errorf("applied tree must be a FIXPOINT of the plan — no managed file should diverge from its just-recorded origin hash; got:\n%s", secondOut)
	}
	if strings.Contains(secondOut, "CONFLICT") {
		t.Errorf("applied tree must leave no armed conflicts behind; got:\n%s", secondOut)
	}
}

// dryRunSectionRe matches a plan section header: an action label ("Would
// SEED", "Would PRESERVE", "CONFLICT", ...) followed by free prose — which may
// itself contain parentheses and em dashes — then a declared path count in
// parentheses and a trailing colon, at column 0. The anchored tail makes the
// captured count the LAST "(N):" in the line, so prose parentheses never
// confuse it.
var dryRunSectionRe = regexp.MustCompile(`^((?:Would [A-Z]+|CONFLICT).*?)\s*\((\d+)\):$`)

// parseDryRunPlanSections extracts the operator-visible path→action mapping
// from a --dry-run plan. Each section header declares its path count; exactly
// that many two-space-indented path lines follow. The declared count — not
// indentation alone — is what delimits paths from the indented advisory prose
// printed after them (e.g. the OVERWRITE remediation block), so a malformed
// plan (count/lines mismatch) fails the test instead of silently mis-parsing.
func parseDryRunPlanSections(t *testing.T, out string) map[string]string {
	t.Helper()
	plan := map[string]string{}
	lines := strings.Split(out, "\n")
	for i := 0; i < len(lines); i++ {
		m := dryRunSectionRe.FindStringSubmatch(lines[i])
		if m == nil {
			continue
		}
		action := m[1]
		count, err := strconv.Atoi(m[2])
		if err != nil {
			t.Fatalf("plan section header %q carries a non-numeric count: %v", lines[i], err)
		}
		got := 0
		for j := i + 1; j < len(lines) && got < count; j++ {
			l := lines[j]
			if !strings.HasPrefix(l, "  ") || strings.TrimSpace(l) == "" {
				break
			}
			p := strings.TrimSpace(l)
			if _, dup := plan[p]; dup {
				t.Errorf("plan lists path %s in more than one section", p)
			}
			plan[p] = action
			got++
		}
		if got != count {
			t.Fatalf("plan section %q declared %d path(s) but only %d well-formed indented line(s) followed; plan output:\n%s", lines[i], count, got, out)
		}
	}
	return plan
}

// walkTreePaths lists all file paths under root (relative), sorted.
func walkTreePaths(t *testing.T, root string) []string {
	t.Helper()
	var out []string
	_ = filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		rel, rerr := filepath.Rel(root, p)
		if rerr != nil {
			return nil
		}
		out = append(out, filepath.ToSlash(rel))
		return nil
	})
	sort.Strings(out)
	return out
}

// TestInstall_ExistingInstallNotBackfilled is the SACRED existing-install
// invariant at the CLI level: a target installed --minimal keeps its minimal
// selection through (a) a bare re-install and (b) an update — the frozen full
// recipe NEVER backfills into an existing live profile (no union of its
// capabilities/overlays, no render of the full surface).
func TestInstall_ExistingInstallNotBackfilled(t *testing.T) {
	root := t.TempDir()
	if _, err := installIntoWithSelector(t, root, false, true, ""); err != nil {
		t.Fatalf("initial --minimal install: %v", err)
	}
	profilePath := filepath.Join(root, harnessProfileName)
	minimalBytes, err := os.ReadFile(profilePath)
	if err != nil {
		t.Fatalf("read minimal profile: %v", err)
	}

	// (a) Bare re-install over the existing install: historical semantics —
	// reconcile, live selection preserved, NO full-recipe backfill.
	out, err := installIntoWithSelector(t, root, false, false, "")
	if err != nil {
		t.Fatalf("bare re-install over existing install: %v (out=%q)", err, out)
	}
	after, err := os.ReadFile(profilePath)
	if err != nil {
		t.Fatalf("re-read profile after re-install: %v", err)
	}
	if string(after) != string(minimalBytes) {
		t.Errorf("bare re-install must NOT change the live profile (no backfill);\n before=%q\n after=%q", minimalBytes, after)
	}
	// The full surface did not render either.
	if p := filepath.Join(root, ".opencode", "agents", "releaser.md"); pathExists(t, p) {
		t.Errorf("bare re-install must not backfill the release pack surface")
	}
	if !strings.Contains(out, "existing live profile (preserved)") {
		t.Errorf("re-install selection report must name the preserved live selection; got:\n%s", out)
	}

	// (b) Update keeps the minimal selection too (recurring reconciliation is
	// untouched by bootstrap selection).
	if _, err := seamUpdateOut(t, root); err != nil {
		t.Fatalf("update after re-install: %v", err)
	}
	afterUpdate, err := os.ReadFile(profilePath)
	if err != nil {
		t.Fatalf("re-read profile after update: %v", err)
	}
	if string(afterUpdate) != string(minimalBytes) {
		t.Errorf("update must NOT change the minimal live profile (no backfill);\n before=%q\n after=%q", minimalBytes, afterUpdate)
	}
	if p := filepath.Join(root, ".opencode", "agents", "releaser.md"); pathExists(t, p) {
		t.Errorf("update must not backfill the release pack surface")
	}
}
