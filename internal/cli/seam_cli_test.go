package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	corpus "github.com/vhqtvn/vh-agent-harness"
	"github.com/vhqtvn/vh-agent-harness/internal/lineage"
	"github.com/vhqtvn/vh-agent-harness/internal/ownership"
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
	// (valid enum), modules=[core,web] (platform adds core), backlog=true.
	// After update the reconciler should keep supervised/web/backlog AND merge in
	// the platform's core module (union-dedup) -> armed-merged.
	userArmed := []byte("profile: supervised\nmodules: [web]\nfeatures:\n  backlog: true\noverlays: [web-overlay]\npolicy_packs: []\n")
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

	// Armed file reconciled: supervised/web retained, core merged in.
	armedAfter, err := os.ReadFile(filepath.Join(root, ".vh-agent-harness/vh-harness-profile.yml"))
	if err != nil {
		t.Fatalf("read armed after: %v", err)
	}
	armedStr := string(armedAfter)
	for _, want := range []string{"supervised", "web", "backlog: true", "web-overlay"} {
		if !strings.Contains(armedStr, want) {
			t.Errorf("armed reconcile lost user value %q; got:\n%s", want, armedStr)
		}
	}
	if !strings.Contains(armedStr, "core") {
		t.Errorf("armed reconcile did not merge platform module 'core'; got:\n%s", armedStr)
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
