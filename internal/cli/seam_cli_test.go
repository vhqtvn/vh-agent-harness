package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	corpus "github.com/vhqtvn/vh-agent-harness"
	"github.com/vhqtvn/vh-agent-harness/internal/lineage"
	"github.com/vhqtvn/vh-agent-harness/internal/ownership"
	"github.com/vhqtvn/vh-agent-harness/internal/substrate"
)

// seamInstallInto runs the seam install path (runInstall) into root, the same
// path the CLI `vh-agent-harness install` verb uses. It is the canonical fixture helper
// for the seam verbs (doctor/update/reconcile).
func seamInstallInto(t *testing.T, root string) {
	t.Helper()
	installFl = newInstallFlags()
	installFl.target = root
	cmd, buf := newOutCmd()
	if err := runInstall(cmd, []string{}); err != nil {
		t.Fatalf("seam install into %s: %v (out=%q)", root, err, buf.String())
	}
}

// seamInstallAnswers is the project_name/project_slug pair seamInstallInto
// records in lineage. The default install flags use name="My Project" and
// slug=<cwd basename> (the temp dir), so the lineage answer digest is computed
// over those. Tests that need a stable digest install with a known cwd or use
// the digest from lineage.Read directly.
func seamDoctorOut(t *testing.T, root string) string {
	t.Helper()
	var out string
	runWithCwd(t, root, func() {
		doctorTargetFlag = root
		defer func() { doctorTargetFlag = "" }()
		cmd, buf := newOutCmd()
		_ = runDoctor(cmd, []string{})
		out = buf.String()
	})
	return out
}

func seamUpdateOut(t *testing.T, root string) (string, error) {
	t.Helper()
	var out string
	var err error
	runWithCwd(t, root, func() {
		updateTargetFlag = root
		defer func() { updateTargetFlag = "" }()
		cmd, buf := newOutCmd()
		err = runUpdate(cmd, []string{})
		out = buf.String()
	})
	return out, err
}

// TestSeamUpdate_FailClosedOnBadOverlay (W9/Q5) confirms the seam refuses to
// render when a profile-listed overlay pack fails to open. Previously this was
// warn-and-skip, which silently produced an INCOMPLETE render (missing the
// agents/commands/skills the operator declared in the profile) with no signal.
// Now it hard-fails naming the exact pack + the underlying error. There is no
// separate "discovered/lenient" overlay category — every overlay is
// profile-listed (activeOverlays reads vh-harness-profile.yml `overlays:` only),
// so fail-closed across the board is the correct, predictable rule.
func TestSeamUpdate_FailClosedOnBadOverlay(t *testing.T) {
	target := t.TempDir()
	seamInstallInto(t, target)
	// Declare a pack that does not exist: no project-local dir, no embedded pack.
	// A schema-valid profile so activeOverlays returns the list (not nil).
	writeProfile(t, target, "profile: minimal\nmodules: [core]\nfeatures:\n  backlog: false\noverlays: [totally-bogus-pack]\npolicy_packs: []\n")
	out, err := seamUpdateOut(t, target)
	if err == nil {
		t.Fatalf("update must FAIL (not warn-and-skip) when a profile-listed overlay won't open; got nil err. out=%q", out)
	}
	if !strings.Contains(err.Error(), "totally-bogus-pack") {
		t.Errorf("error must name the failing pack; got: %v", err)
	}
	if !strings.Contains(err.Error(), "refusing to render") {
		t.Errorf("error must explain the refusal; got: %v", err)
	}
}

// --- seam install: lineage -----------------------------------------------

// TestSeamInstall_WritesLineage confirms the seam install path writes a lineage
// record that records the embed-fs renderer and the harness/<version> ref (the
// seam's content-origin tag), distinct from the legacy shim lineage.
func TestSeamInstall_WritesLineage(t *testing.T) {
	root := t.TempDir()
	seamInstallInto(t, root)
	lin, err := lineage.Read(root)
	if err != nil {
		t.Fatalf("seam lineage unreadable: %v", err)
	}
	if lin == nil {
		t.Fatal("seam install did not write lineage")
	}
	if lin.Render.RenderedBy != "embed-fs" {
		t.Errorf("rendered_by: got %q want %q", lin.Render.RenderedBy, "embed-fs")
	}
	if want := "harness/" + Version; lin.Template.Ref != want {
		t.Errorf("template.ref: got %q want %q", lin.Template.Ref, want)
	}
	if lin.Template.Source != corpus.CoreDir {
		t.Errorf("template.source: got %q want %q", lin.Template.Source, corpus.CoreDir)
	}
}

// --- seam install: seeds armed + owned -----------------------------------

// TestSeamInstall_SeedsArmedAndOwned confirms the first seam install seeds the
// platform_armed (vh-harness-profile.yml) and project_owned
// (forbidden-patterns.project.js) files from the platform defaults, and that both
// are present on disk.
func TestSeamInstall_SeedsArmedAndOwned(t *testing.T) {
	root := t.TempDir()
	seamInstallInto(t, root)
	for _, rel := range []string{".vh-agent-harness/vh-harness-profile.yml", ".opencode/repo-configs/forbidden-patterns.project.js"} {
		abs := filepath.Join(root, rel)
		if _, err := os.Stat(abs); err != nil {
			t.Errorf("seam install did not seed %s: %v", rel, err)
		}
	}
}

// --- seam doctor: HEALTHY after install ----------------------------------

func TestSeamDoctor_HealthyAfterInstall(t *testing.T) {
	root := t.TempDir()
	seamInstallInto(t, root)
	out := seamDoctorOut(t, root)
	if !strings.Contains(out, "result: HEALTHY") {
		t.Errorf("seam doctor want HEALTHY after install, got %q", out)
	}
	if !strings.Contains(out, "managed-drift") {
		t.Errorf("seam doctor want managed-drift section, got %q", out)
	}
	if !strings.Contains(out, "armed-schema") {
		t.Errorf("seam doctor want armed-schema section, got %q", out)
	}
}

// TestSeamDoctor_HealthyAfterInstallWithCustomAnswers is the regression guard
// for the lineage AnswersRef.Values answer-recovery path. The default install
// uses name="My Project" (already != a temp dir basename), but this test makes
// the divergence explicit and undeniable: it installs with a project_name/slug
// deliberately distinct from the target dir basename, then confirms doctor is
// HEALTHY. Without answer recovery, every harness-token-bearing managed file
// ({PROJECT_NAME}/{PROJECT_SLUG}/{COORDINATOR_DIR}) would false-flag managed
// drift because doctor would re-render with the dir-basename default instead of
// the operator's chosen install identity.
func TestSeamDoctor_HealthyAfterInstallWithCustomAnswers(t *testing.T) {
	root := t.TempDir()
	installFl = newInstallFlags()
	installFl.target = root
	installFl.name = "Custom Project Name"
	installFl.slug = "custom-project-slug"
	// Sanity: the install identity must actually differ from the dir basename,
	// else the test is not exercising the recovery path.
	if installFl.name == filepath.Base(root) || installFl.slug == filepath.Base(root) {
		t.Fatalf("test setup: install identity collides with dir basename %q", filepath.Base(root))
	}
	cmd, _ := newOutCmd()
	if err := runInstall(cmd, []string{}); err != nil {
		t.Fatalf("seam install with custom answers: %v", err)
	}
	out := seamDoctorOut(t, root)
	if !strings.Contains(out, "result: HEALTHY") {
		t.Errorf("doctor want HEALTHY with custom install answers (recovered from lineage), got %q", out)
	}
}

// --- seam doctor: UNHEALTHY on managed drift -----------------------------

func TestSeamDoctor_UnhealthyOnManagedDrift(t *testing.T) {
	root := t.TempDir()
	seamInstallInto(t, root)
	// Corrupt a managed file the classifier tags platform_managed.
	const rel = ".vh-agent-harness/AGENTS.core.md"
	if err := os.WriteFile(filepath.Join(root, rel), []byte("CORRUPTED\n"), 0o644); err != nil {
		t.Fatalf("corrupt %s: %v", rel, err)
	}
	out := seamDoctorOut(t, root)
	if !strings.Contains(out, "UNHEALTHY") {
		t.Errorf("want UNHEALTHY on managed drift, got %q", out)
	}
	if !strings.Contains(out, "drifted") {
		t.Errorf("want drift detail, got %q", out)
	}
}

// --- seam doctor: UNHEALTHY on armed-schema-invalid ----------------------

func TestSeamDoctor_UnhealthyOnArmedSchemaInvalid(t *testing.T) {
	root := t.TempDir()
	seamInstallInto(t, root)
	// Poison the armed file with an invalid profile enum value.
	invalid := []byte("profile: experimental_not_in_enum\nmodules: [core]\nfeatures:\n  backlog: false\noverlays: []\npolicy_packs: []\n")
	if err := os.WriteFile(filepath.Join(root, ".vh-agent-harness/vh-harness-profile.yml"), invalid, 0o644); err != nil {
		t.Fatalf("poison armed: %v", err)
	}
	out := seamDoctorOut(t, root)
	if !strings.Contains(out, "UNHEALTHY") {
		t.Errorf("want UNHEALTHY on armed-schema-invalid, got %q", out)
	}
	if !strings.Contains(out, "armed-schema") {
		t.Errorf("want armed-schema section, got %q", out)
	}
}

// --- seam doctor: UNHEALTHY on lineage authority leak -------------------

func TestSeamDoctor_UnhealthyOnLineageAuthorityLeak(t *testing.T) {
	root := t.TempDir()
	seamInstallInto(t, root)
	// Overwrite lineage with a poisoned file leaking S3 (profile) + S4 (services).
	poisoned := []byte("lineage_version: \"1\"\ntemplate: {source: x}\n" +
		"copier: {version: \"\"}\nanswers: {digest: sha256:x}\n" +
		"render: {last_successful_update_id: x}\n" +
		"profile: minimal\nservices: {web: {}}\n")
	if err := os.WriteFile(lineage.FilePath(root), poisoned, 0o644); err != nil {
		t.Fatal(err)
	}
	out := seamDoctorOut(t, root)
	if !strings.Contains(out, "lineage unreadable") {
		t.Errorf("want lineage unreadable FAIL, got %q", out)
	}
	if !strings.Contains(out, "UNHEALTHY") {
		t.Errorf("want UNHEALTHY on authority leak, got %q", out)
	}
}

// --- seam update: preserves owned + reconciles armed --------------------

// TestSeamUpdate_PreservesOwnedReconcilesArmed mirrors the render-apply proof
// phases: install -> edit owned (must survive) + edit armed to a clean mergeable
// state + corrupt managed -> update -> managed refreshed, owned byte-identical,
// armed RECONCILED (project additions retained + platform defaults merged).
func TestSeamUpdate_PreservesOwnedReconcilesArmed(t *testing.T) {
	root := t.TempDir()
	seamInstallInto(t, root)

	// Snapshot the seeded owned file bytes (forbidden-patterns.project.js).
	ownedRel := ".opencode/repo-configs/forbidden-patterns.project.js"
	ownedBefore, err := os.ReadFile(filepath.Join(root, ownedRel))
	if err != nil {
		t.Fatalf("read owned before: %v", err)
	}
	// User edits the owned file (must be preserved byte-for-byte across update).
	userOwned := append([]byte("# user-added line\n"), ownedBefore...)
	if err := os.WriteFile(filepath.Join(root, ownedRel), userOwned, 0o644); err != nil {
		t.Fatalf("edit owned: %v", err)
	}

	// User edits the armed file to a CLEAN mergeable state: profile=supervised
	// (valid enum), modules=[web] (deprecated; the platform default no longer
	// ships a `modules:` block — removed once `profile:` presets replaced it — so
	// the append-only reconcile union contributes nothing and the user's [web]
	// survives as-is), backlog=true. After update the reconciler should keep
	// supervised/web/backlog -> armed-merged. NOTE: overlays:[] — the seam is
	// now fail-closed on a profile-listed overlay that won't open (W9/Q5), so a
	// fixture must not declare a non-existent pack (the old web-overlay was
	// relocated out of the shipped set; see overlay.KnownPacks).
	userArmed := []byte("profile: supervised\nmodules: [web]\nfeatures:\n  backlog: true\noverlays: []\npolicy_packs: []\n")
	if err := os.WriteFile(filepath.Join(root, ".vh-agent-harness/vh-harness-profile.yml"), userArmed, 0o644); err != nil {
		t.Fatalf("edit armed: %v", err)
	}

	// Corrupt a managed file (must be refreshed on update).
	const managedRel = ".vh-agent-harness/AGENTS.core.md"
	if err := os.WriteFile(filepath.Join(root, managedRel), []byte("OLD V1 CORRUPT\n"), 0o644); err != nil {
		t.Fatalf("corrupt managed: %v", err)
	}

	out, err := seamUpdateOut(t, root)
	if err != nil {
		t.Fatalf("seam update: %v (out=%q)", err, out)
	}
	if !strings.Contains(out, "armed-merged") {
		t.Errorf("update want armed-merged outcome, got %q", out)
	}
	if !strings.Contains(out, "managed-overwrite") {
		t.Errorf("update want managed-overwrite outcome, got %q", out)
	}

	// Owned file preserved byte-for-byte (user edit survives).
	ownedAfter, err := os.ReadFile(filepath.Join(root, ownedRel))
	if err != nil {
		t.Fatalf("read owned after: %v", err)
	}
	if string(ownedAfter) != string(userOwned) {
		t.Errorf("owned file NOT preserved byte-for-byte across update")
	}

	// Armed file reconciled: supervised/web/backlog retained. The platform default
	// no longer ships a `modules:` block (deprecated in Phase 5 and removed from
	// the embedded default), so the armed reconcile's append-only union contributes
	// nothing to `modules:` — the user's `modules: [web]` survives as-is (the
	// deprecated field is preserved through reconcile, not added to).
	armedAfter, err := os.ReadFile(filepath.Join(root, ".vh-agent-harness/vh-harness-profile.yml"))
	if err != nil {
		t.Fatalf("read armed after: %v", err)
	}
	armedStr := string(armedAfter)
	for _, want := range []string{"supervised", "web", "backlog: true"} {
		if !strings.Contains(armedStr, want) {
			t.Errorf("armed reconcile lost user value %q; got:\n%s", want, armedStr)
		}
	}
	// Doctor must be HEALTHY now (armed is schema-valid after reconcile).
	docOut := seamDoctorOut(t, root)
	if !strings.Contains(docOut, "result: HEALTHY") {
		t.Errorf("doctor want HEALTHY after clean reconcile, got %q", docOut)
	}
}

// --- seam update: armed conflict -> proposal ---------------------------

// TestSeamUpdate_ArmedConflictEmitsProposal confirms a needs-decision conflict
// (a profile value the platform's enum has withdrawn) leaves the project
// instance untouched and reports a structured proposal (no conflict markers).
func TestSeamUpdate_ArmedConflictEmitsProposal(t *testing.T) {
	root := t.TempDir()
	seamInstallInto(t, root)

	// Set an invalid profile the platform enum does not recognize.
	conflict := []byte("profile: experimental\nmodules: [core]\nfeatures:\n  backlog: false\noverlays: []\npolicy_packs: []\n")
	if err := os.WriteFile(filepath.Join(root, ".vh-agent-harness/vh-harness-profile.yml"), conflict, 0o644); err != nil {
		t.Fatalf("set conflict armed: %v", err)
	}
	before, _ := os.ReadFile(filepath.Join(root, ".vh-agent-harness/vh-harness-profile.yml"))

	out, _ := seamUpdateOut(t, root)
	if !strings.Contains(out, "proposal") && !strings.Contains(out, "PROPOSAL") {
		t.Errorf("update want proposal outcome, got %q", out)
	}

	// Project instance left untouched (no conflict markers, byte-identical).
	after, _ := os.ReadFile(filepath.Join(root, ".vh-agent-harness/vh-harness-profile.yml"))
	if string(after) != string(before) {
		t.Errorf("armed conflict must leave project instance untouched; it changed")
	}
	if strings.Contains(string(after), "<<<<") || strings.Contains(string(after), ">>>>") {
		t.Errorf("armed conflict must NOT drop conflict markers; found them")
	}
}

// --- seam install: idempotent re-install -------------------------------

func TestSeamInstall_IdempotentReinstall(t *testing.T) {
	root := t.TempDir()
	seamInstallInto(t, root)
	// Re-install must be clean (no error; armed-merged or armed-noop, no proposals).
	out2 := ""
	installFl = newInstallFlags()
	installFl.target = root
	cmd, buf := newOutCmd()
	if err := runInstall(cmd, []string{}); err != nil {
		t.Fatalf("re-install: %v (out=%q)", err, buf.String())
	}
	out2 = buf.String()
	if strings.Contains(out2, "PROPOSAL") {
		t.Errorf("idempotent re-install must not emit proposals, got %q", out2)
	}
	// Lineage still parseable and embed-fs rendered.
	lin, err := lineage.Read(root)
	if err != nil {
		t.Fatalf("re-install lineage unreadable: %v", err)
	}
	if lin.Render.RenderedBy != "embed-fs" {
		t.Errorf("re-install rendered_by: got %q want embed-fs", lin.Render.RenderedBy)
	}
}

// TestSeamReinstall_ReportsManagedUnchangedOnCurrentTree verifies the user-
// facing summary distinguishes already-current managed files: a re-install over
// a tree whose managed files are byte-identical to the corpus reports
// managed-unchanged (not managed-overwrite) for those files, while a drifted
// managed file still reports managed-overwrite.
func TestSeamReinstall_ReportsManagedUnchangedOnCurrentTree(t *testing.T) {
	root := t.TempDir()
	seamInstallInto(t, root)

	// Corrupt exactly one managed file; the rest stay byte-identical to the corpus.
	const driftedRel = ".vh-agent-harness/AGENTS.core.md"
	if err := os.WriteFile(filepath.Join(root, driftedRel), []byte("CORRUPTED\n"), 0o644); err != nil {
		t.Fatalf("corrupt managed: %v", err)
	}

	installFl = newInstallFlags()
	installFl.target = root
	cmd, buf := newOutCmd()
	if err := runInstall(cmd, []string{}); err != nil {
		t.Fatalf("re-install: %v (out=%q)", err, buf.String())
	}
	out := buf.String()

	// The drifted file -> managed-overwrite.
	if !strings.Contains(out, "managed-overwrite") {
		t.Errorf("want managed-overwrite for the drifted file, got %q", out)
	}
	// The byte-identical majority -> managed-unchanged.
	if !strings.Contains(out, "managed-unchanged") {
		t.Errorf("want managed-unchanged for already-current managed files, got %q", out)
	}
}

// --- classifier reads S2 manifest (smoke) ------------------------------

// TestSeamClassifier_ReadsCoreOwnership confirms the seam classifier is built
// from corpus.CoreOwnershipDefaults and classifies the armed/owned exceptions
// correctly (the rest being platform_managed). This is the #2 deliverable: the
// seam's Classifier reads the S2 ownership manifest.
func TestSeamClassifier_ReadsCoreOwnership(t *testing.T) {
	cls, err := seamClassifierImpl()
	if err != nil {
		t.Fatalf("seamClassifierImpl: %v", err)
	}
	cases := []struct {
		path string
		want ownership.Class
	}{
		{".vh-agent-harness/vh-harness-profile.yml", ownership.ClassPlatformArmed},
		{".opencode/repo-configs/forbidden-patterns.project.js", ownership.ClassProjectOwned},
		{".vh-agent-harness/config-transform.mjs", ownership.ClassProjectOwned},
		{".vh-agent-harness/config-transform.core.mjs", ownership.ClassPlatformManaged},
		{".vh-agent-harness/AGENTS.core.md", ownership.ClassPlatformManaged},
		{".opencode/agents/build.md", ownership.ClassPlatformManaged},
		{".opencode/skills/gated-commit/SKILL.md", ownership.ClassPlatformManaged},
	}
	for _, c := range cases {
		got, ok := cls.Classify(c.path)
		if !ok {
			t.Errorf("Classify(%q): not classified (fail-closed)", c.path)
			continue
		}
		if got.Class != c.want {
			t.Errorf("Classify(%q): got %q want %q", c.path, got.Class, c.want)
		}
	}
}

// --- canonical permission emission (O5 slice 2b) ---------------------------

// TestSeamInstall_EmitsCanonicalPermissions verifies the Go-native permission
// emitter (internal/permconfig) is wired into renderSeamStaging. After a clean
// install:
//   - opencode.jsonc is in canonical normalized form (no // MANAGED / // PROJECT:
//     comments; Q1b deterministic JSONC).
//   - features.backlog=true (the default install) → the normalize-backlog.js
//     permission entry IS present (the resolver wipe bug is fixed by the Go
//     emitter).
//   - allowed-commands.js is generated from Go canonical tables with the JS
//     module export shape the shell-guard runtime hook imports.
//   - doctor passes HEALTHY immediately (managed-drift is byte-stable across
//     update→doctor because both re-render via the same renderSeamStaging).
func TestSeamInstall_EmitsCanonicalPermissions(t *testing.T) {
	root := t.TempDir()
	seamInstallInto(t, root)

	cfg, err := os.ReadFile(filepath.Join(root, "opencode.jsonc"))
	if err != nil {
		t.Fatalf("read opencode.jsonc: %v", err)
	}
	cfgStr := string(cfg)
	if strings.Contains(cfgStr, "// MANAGED") || strings.Contains(cfgStr, "// PROJECT:") {
		t.Errorf("canonical opencode.jsonc must NOT carry comments (Q1b); found // MANAGED or // PROJECT:")
	}
	if !strings.Contains(cfgStr, "normalize-backlog.js") {
		t.Errorf("features.backlog=true (default install) must include normalize-backlog permission; the resolver wipe bug is fixed by the Go emitter")
	}

	ac, err := os.ReadFile(filepath.Join(root, ".opencode", "repo-configs", "allowed-commands.js"))
	if err != nil {
		t.Fatalf("read allowed-commands.js: %v", err)
	}
	acStr := string(ac)
	if !strings.Contains(acStr, "export const COMMANDS = {") {
		t.Errorf("allowed-commands.js must have the JS module export shape shell-guard imports; got prefix %q", acStr[:min(60, len(acStr))])
	}
	if !strings.HasPrefix(acStr, "// GENERATED by") {
		t.Errorf("allowed-commands.js must start with a GENERATED header marking it as non-editable")
	}
	for _, key := range []string{"readonly:", "git_readonly:", "gate:"} {
		if !strings.Contains(acStr, key) {
			t.Errorf("allowed-commands.js must declare group %q", key)
		}
	}

	out := seamDoctorOut(t, root)
	if !strings.Contains(out, "result: HEALTHY") {
		t.Errorf("doctor want HEALTHY after install with Go-emitted permissions, got %q", out)
	}
}

// TestSeamUpdate_BacklogFeatureOffExcludesPermission verifies the Go emitter
// respects features.backlog=false: the normalize-backlog.js permission entry
// must be ABSENT from opencode.jsonc, and doctor must still pass HEALTHY (no
// false drift from the conditional collapsing).
func TestSeamUpdate_BacklogFeatureOffExcludesPermission(t *testing.T) {
	root := t.TempDir()
	seamInstallInto(t, root)
	writeProfile(t, root, "profile: minimal\nmodules: [core]\nfeatures:\n  backlog: false\noverlays: []\npolicy_packs: []\n")
	if _, err := seamUpdateOut(t, root); err != nil {
		t.Fatalf("update with backlog=false: %v", err)
	}
	cfg, err := os.ReadFile(filepath.Join(root, "opencode.jsonc"))
	if err != nil {
		t.Fatalf("read opencode.jsonc: %v", err)
	}
	if strings.Contains(string(cfg), "normalize-backlog.js") {
		t.Errorf("features.backlog=false must EXCLUDE normalize-backlog permission; still present after update")
	}
	out := seamDoctorOut(t, root)
	if !strings.Contains(out, "result: HEALTHY") {
		t.Errorf("doctor want HEALTHY with backlog=false, got %q", out)
	}
}

// TestSeamInstall_GateExemptAgentOmitsGateCommands verifies a gate-exempt agent
// (build) has NO gate command entries in its permission.bash block, while the
// sole gate-enabled agent (committer) DOES have gate entries with "allow". This
// is the OpenCode deriveSubagentSessionPermission correctness contract: a parent
// gate deny would bleed into the committer subagent and block commits.
//
// Phase 5: the bare install renders the embedded-default `profile: minimal`
// (baseline-only — no committer). This test needs the gated-commit cluster, so
// it switches the profile to `supervised` and updates before asserting. The
// gate-exempt/gate-enabled contract under test is unchanged; only the profile
// setup is Phase-5-aware.
func TestSeamInstall_GateExemptAgentOmitsGateCommands(t *testing.T) {
	root := t.TempDir()
	seamInstallInto(t, root)
	// Render the gated-commit cluster (committer et al.) via the supervised preset.
	writeProfile(t, root, "profile: supervised\nfeatures:\n  backlog: true\noverlays: []\npolicy_packs: []\n")
	if _, err := seamUpdateOut(t, root); err != nil {
		t.Fatalf("update with profile:supervised: %v", err)
	}
	cfg, err := os.ReadFile(filepath.Join(root, "opencode.jsonc"))
	if err != nil {
		t.Fatalf("read opencode.jsonc: %v", err)
	}
	// The canonical output is valid JSON (comments dropped per Q1b), so we can
	// parse it directly and navigate to specific agent permission blocks.
	var doc struct {
		Agent map[string]struct {
			Permission struct {
				Bash map[string]string `json:"bash"`
			} `json:"permission"`
		} `json:"agent"`
	}
	if err := json.Unmarshal(cfg, &doc); err != nil {
		t.Fatalf("unmarshal canonical opencode.jsonc: %v", err)
	}

	// build is gate-exempt → must NOT contain any commit-gate MUTATION entries.
	// (.opencode/scripts/commit-gate.sh status is a pure-read metadata probe and
	// lives in the readonly group, so build legitimately carries it with "allow"
	// — Q2 split. Only mutation verbs stay gate-grouped.)
	buildBash := doc.Agent["build"].Permission.Bash
	if buildBash == nil {
		t.Fatal("build agent permission.bash block is missing")
	}
	for cmd := range buildBash {
		if !strings.Contains(cmd, "commit-gate.sh") {
			continue
		}
		if strings.HasSuffix(cmd, " status") {
			continue // read-only status check; allowed for all agents
		}
		t.Errorf("build agent (gate-exempt) must NOT have gate mutation entries; found %q in its permission.bash", cmd)
	}

	// committer is the ONLY gate-enabled agent → must have commit-gate entries
	// with "allow".
	committerBash := doc.Agent["committer"].Permission.Bash
	if committerBash == nil {
		t.Fatal("committer agent permission.bash block is missing")
	}
	foundGateAllow := false
	for cmd, decision := range committerBash {
		if strings.Contains(cmd, "commit-gate.sh") && decision == "allow" {
			foundGateAllow = true
			break
		}
	}
	if !foundGateAllow {
		t.Errorf("committer agent (gate-enabled) must have at least one commit-gate.sh entry with 'allow'")
	}
}

// --- migration detection (O5 slice 2c, Q5c) ---------------------------------

// TestMigration_AllowedCommandsCustomized verifies isAllowedCommandsCustomized
// detects when the live allowed-commands.js differs from the generated form.
// This is the Q5c migration courtesy check: never silently overwrite a
// customized file without warning the operator.
func TestMigration_AllowedCommandsCustomized(t *testing.T) {
	root := t.TempDir()
	seamInstallInto(t, root)

	// After a clean install, the live file must match the generated form → NOT
	// customized. We verify this by rendering fresh staging and comparing.
	staging := t.TempDir()
	sub, err := coreSubFSImpl()
	if err != nil {
		t.Fatalf("coreSubFSImpl: %v", err)
	}
	renderer := substrate.EmbedFSRenderer{Source: sub}
	answers := mergeRenderAnswers(installRenderAnswers(root), readProfileAnswers(root))
	if _, _, err := renderSeamStaging(staging, renderer, answers, root); err != nil {
		t.Fatalf("render staging: %v", err)
	}
	if isAllowedCommandsCustomized(root, staging) {
		t.Errorf("clean install must NOT flag allowed-commands.js as customized")
	}

	// Now corrupt the live file — simulate an operator who added a custom command.
	acPath := filepath.Join(root, ".opencode", "repo-configs", "allowed-commands.js")
	if err := os.WriteFile(acPath, []byte("// my custom edits\nexport const COMMANDS = { custom: [] };\n"), 0o644); err != nil {
		t.Fatalf("customize allowed-commands.js: %v", err)
	}
	if !isAllowedCommandsCustomized(root, staging) {
		t.Errorf("customized allowed-commands.js must be DETECTED as customized")
	}
}

// TestMigration_UpdateOverwritesCustomizedAllowedCommands verifies that update
// proceeds (non-blocking) and overwrites a customized allowed-commands.js with
// the canonical form. The migration warning is non-blocking (the file is
// platform_managed); the operator is warned on stderr but the update succeeds.
func TestMigration_UpdateOverwritesCustomizedAllowedCommands(t *testing.T) {
	root := t.TempDir()
	seamInstallInto(t, root)

	// Customize the live file.
	acPath := filepath.Join(root, ".opencode", "repo-configs", "allowed-commands.js")
	custom := []byte("// my custom edits\nexport const COMMANDS = { custom: [] };\n")
	if err := os.WriteFile(acPath, custom, 0o644); err != nil {
		t.Fatalf("customize allowed-commands.js: %v", err)
	}

	// Update must succeed (non-blocking warning).
	if _, err := seamUpdateOut(t, root); err != nil {
		t.Fatalf("update with customized allowed-commands.js must succeed (non-blocking warning): %v", err)
	}

	// After update, the file must be the canonical generated form.
	ac, err := os.ReadFile(acPath)
	if err != nil {
		t.Fatalf("read allowed-commands.js after update: %v", err)
	}
	if !strings.Contains(string(ac), "// GENERATED by") {
		t.Errorf("customized allowed-commands.js must be overwritten with canonical GENERATED form after update")
	}
	if strings.Contains(string(ac), "my custom edits") {
		t.Errorf("custom content must NOT survive the canonical overwrite")
	}

	// Doctor must still pass.
	out := seamDoctorOut(t, root)
	if !strings.Contains(out, "result: HEALTHY") {
		t.Errorf("doctor want HEALTHY after update overwrote customized file, got %q", out)
	}
}
