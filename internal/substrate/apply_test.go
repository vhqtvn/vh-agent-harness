package substrate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	corpus "github.com/vhqtvn/vh-agent-harness" // root package: embed + CoreOwnershipDefaults
	"github.com/vhqtvn/vh-agent-harness/internal/lineage"
	"github.com/vhqtvn/vh-agent-harness/internal/ownership"
)

// corpusRoot is the Slice 1 minimal corpus (1 managed + 1 armed + 1 owned),
// relative to this package. The seam tests render it via FixtureRenderer so the
// committed corpus is actually exercised, not a parallel test fixture.
const corpusRoot = "../../templates/core"

// corpusClassifier maps the curated core corpus to its S2 ownership classes via
// the real ownership.Resolve (exact-path, raise-only) path. The defaults are
// derived by walking the EMBEDDED core corpus (corpus.CoreOwnershipDefaults): every
// path is platform_managed except the documented armed/owned exceptions
// (vh-harness-profile.yml=platform_armed, forbidden-patterns.project.js=project_owned).
// This makes the seam tests robust to corpus growth (Slice 2 widened the corpus
// from 3 files to the full curated set) and proves the S2 manifest mechanism.
func corpusClassifier(t *testing.T) *Classifier {
	t.Helper()
	defaults, err := corpus.CoreOwnershipDefaults()
	if err != nil {
		t.Fatalf("corpus.CoreOwnershipDefaults: %v", err)
	}
	if len(defaults) == 0 {
		t.Fatalf("core ownership defaults are empty")
	}
	eff, err := ownership.Resolve(defaults, nil)
	if err != nil {
		t.Fatalf("ownership.Resolve: %v", err)
	}
	return NewClassifier(eff, nil)
}

// writeFile is a tiny test helper.
func writeFile(t *testing.T, dir, rel, content string) {
	t.Helper()
	p := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", p, err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", p, err)
	}
}

func readFile(t *testing.T, dir, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, rel))
	if err != nil {
		t.Fatalf("read %s: %v", filepath.Join(dir, rel), err)
	}
	return string(b)
}

func TestApply_ManagedUpdatedOwnedPreservedArmedReconciled(t *testing.T) {
	live := t.TempDir()
	staging := t.TempDir()

	// --- Seed the LIVE tree (the project's current state before update) ---
	// managed: OLD platform content (must be overwritten by staging).
	writeFile(t, live, ".vh-agent-harness/AGENTS.core.md", "# AGENTS.core.md\nOLD managed content v1\n")
	// owned: USER content (must be PRESERVED across update, never clobbered).
	writeFile(t, live, ".opencode/repo-configs/forbidden-patterns.project.js",
		`{"_comment":"USER-EDITED owned content; must survive update","profile":"supervised","operator":"alice"}`)
	// armed: USER-EDITED profile (project added a module + flipped backlog).
	// Platform default (in corpus) is profile=minimal, modules=[core], backlog=false.
	writeFile(t, live, ".vh-agent-harness/vh-harness-profile.yml", strings.Join([]string{
		"profile: supervised",
		"modules:",
		"  - core",
		"  - web",
		"features:",
		"  backlog: true",
		"overlays:",
		"  - web-overlay",
		"policy_packs: []",
		"",
	}, "\n"))

	// --- Render the corpus into staging via the faithful FixtureRenderer ---
	r := FixtureRenderer{TemplateRoot: corpusRoot}
	if err := r.Render(staging, RenderSpec{
		TemplateSource: "templates/core",
		Answers:        map[string]string{"profile": "supervised"},
	}); err != nil {
		t.Fatalf("render: %v", err)
	}

	// --- Run the seam ---
	report, err := Apply(r, ApplyOptions{
		ProjectRoot:    live,
		StagingDir:     staging,
		Classifier:     corpusClassifier(t),
		HarnessVersion: "0.1.0-slice1",
		TemplateSource: "templates/core",
		Answers:        map[string]string{"profile": "supervised"},
	})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}

	byAction := map[FileAction]FileOutcome{}
	for _, o := range report.Outcomes {
		byAction[o.Action] = o
	}

	// --- platform_managed: UPDATED from staging (byte-identical to staged copy) ---
	if _, ok := byAction[ActionManagedOverwrite]; !ok {
		t.Fatalf("expected a managed-overwrite outcome; got %+v", report.Outcomes)
	}
	stagedManaged, _ := os.ReadFile(filepath.Join(staging, ".vh-agent-harness/AGENTS.core.md"))
	got := readFile(t, live, ".vh-agent-harness/AGENTS.core.md")
	if string(stagedManaged) != got {
		t.Fatalf("managed file not updated to staged content (managed must be byte-identical to staging)")
	}

	// --- project_owned: PRESERVED (user content intact, NOT staging content) ---
	pres, ok := byAction[ActionProjectPreserved]
	if !ok {
		t.Fatalf("expected a project-preserved outcome; got %+v", report.Outcomes)
	}
	if pres.Class != ownership.ClassProjectOwned {
		t.Fatalf("preserved file class: want project_owned, got %s", pres.Class)
	}
	gotOwned := readFile(t, live, ".opencode/repo-configs/forbidden-patterns.project.js")
	if !strings.Contains(gotOwned, "USER-EDITED owned content; must survive update") {
		t.Fatalf("project_owned file was clobbered! got:\n%s", gotOwned)
	}
	if strings.Contains(gotOwned, "REPLACE_ME") {
		t.Fatalf("project_owned file took the staging/seeded content; got:\n%s", gotOwned)
	}

	// --- platform_armed: RECONCILED (user edits retained + platform default present) ---
	merge, ok := byAction[ActionArmedMerged]
	if !ok {
		t.Fatalf("expected an armed-merged outcome; got %+v", report.Outcomes)
	}
	if merge.Class != ownership.ClassPlatformArmed {
		t.Fatalf("armed file class: want platform_armed, got %s", merge.Class)
	}
	gotArmed := readFile(t, live, ".vh-agent-harness/vh-harness-profile.yml")
	// project selection retained (supervised, within enum).
	if !strings.Contains(gotArmed, "profile: supervised") {
		t.Fatalf("armed reconcile lost project's profile selection; got:\n%s", gotArmed)
	}
	// project's module 'web' retained AND it's there.
	if !strings.Contains(gotArmed, "- web") {
		t.Fatalf("armed reconcile lost project's 'web' module; got:\n%s", gotArmed)
	}
	// project's backlog=true override retained.
	if !strings.Contains(gotArmed, "backlog: true") {
		t.Fatalf("armed reconcile lost project's backlog override; got:\n%s", gotArmed)
	}
	// project's web-overlay overlay retained.
	if !strings.Contains(gotArmed, "- web-overlay") {
		t.Fatalf("armed reconcile lost project's overlay; got:\n%s", gotArmed)
	}

	// --- lineage.yml WRITTEN (S1 authority) ---
	lin, err := lineage.Read(live)
	if err != nil {
		t.Fatalf("read lineage: %v", err)
	}
	if lin == nil {
		t.Fatalf("lineage.yml was not written")
	}
	if lin.Render.RenderedBy != "fixture-test-renderer" {
		t.Fatalf("lineage rendered_by: want fixture-test-renderer, got %q", lin.Render.RenderedBy)
	}
	if lin.Answers.Digest == "" {
		t.Fatalf("lineage missing answer digest")
	}
	if report.LineagePath != lineage.FilePath(live) {
		t.Fatalf("report lineage path mismatch: %s != %s", report.LineagePath, lineage.FilePath(live))
	}
}

func TestApply_ArmedConflictEmitsStructuredProposal(t *testing.T) {
	live := t.TempDir()
	staging := t.TempDir()

	// Project selected a profile the platform enum no longer offers -> conflict.
	writeFile(t, live, ".vh-agent-harness/vh-harness-profile.yml", strings.Join([]string{
		"profile: experimental", // NOT in enum {minimal,coordination,supervised,web}
		"modules: [core]",
		"",
	}, "\n"))

	r := FixtureRenderer{TemplateRoot: corpusRoot}
	if err := r.Render(staging, RenderSpec{TemplateSource: "templates/core"}); err != nil {
		t.Fatalf("render: %v", err)
	}

	report, err := Apply(r, ApplyOptions{
		ProjectRoot:    live,
		StagingDir:     staging,
		Classifier:     corpusClassifier(t),
		HarnessVersion: "0.1.0-slice1",
		TemplateSource: "templates/core",
	})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}

	// Exactly one armed-proposal outcome, for vh-harness-profile.yml.
	var prop FileOutcome
	count := 0
	for _, o := range report.Outcomes {
		if o.Action == ActionArmedProposal {
			count++
			prop = o
		}
	}
	if count != 1 || prop.Path != ".vh-agent-harness/vh-harness-profile.yml" {
		t.Fatalf("expected one armed-proposal for vh-harness-profile.yml; got count=%d %+v", count, prop)
	}
	if len(prop.Proposals) != 1 {
		t.Fatalf("expected 1 proposal, got %d", len(prop.Proposals))
	}
	p := prop.Proposals[0]
	if p.Field != "profile" {
		t.Fatalf("proposal field: want 'profile', got %q", p.Field)
	}
	if p.Kind != "enum_removed" {
		t.Fatalf("proposal kind: want enum_removed, got %q", p.Kind)
	}
	if p.ProjectValue != "experimental" {
		t.Fatalf("proposal project value: want experimental, got %v", p.ProjectValue)
	}

	// The project's armed instance must be LEFT UNTOUCHED (no partial write, no
	// conflict markers dropped into it).
	got := readFile(t, live, ".vh-agent-harness/vh-harness-profile.yml")
	if !strings.Contains(got, "profile: experimental") {
		t.Fatalf("conflict must leave project instance untouched; got:\n%s", got)
	}
	if strings.Contains(got, "<<<<") || strings.Contains(got, ".rej") {
		t.Fatalf("seam must NOT drop textual conflict markers into the file; got:\n%s", got)
	}
}

func TestApply_ProjectOwnedSeededWhenAbsent(t *testing.T) {
	live := t.TempDir() // empty: no forbidden-patterns.project.js yet
	staging := t.TempDir()

	r := FixtureRenderer{TemplateRoot: corpusRoot}
	if err := r.Render(staging, RenderSpec{TemplateSource: "templates/core"}); err != nil {
		t.Fatalf("render: %v", err)
	}
	report, err := Apply(r, ApplyOptions{
		ProjectRoot: live, StagingDir: staging,
		Classifier: corpusClassifier(t), HarnessVersion: "0.1.0",
		TemplateSource: "templates/core",
	})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	seeded := false
	for _, o := range report.Outcomes {
		if o.Path == ".opencode/repo-configs/forbidden-patterns.project.js" && o.Action == ActionProjectSeeded {
			seeded = true
		}
	}
	if !seeded {
		t.Fatalf("expected project-seeded for absent owned file; got %+v", report.Outcomes)
	}
	// Seeded content must equal the platform default (staged copy). Content-
	// agnostic: proves the owned file was seeded FROM staging, regardless of what
	// the real forbidden-patterns.project.js happens to contain.
	stagedOwned, _ := os.ReadFile(filepath.Join(staging, ".opencode/repo-configs/forbidden-patterns.project.js"))
	got := readFile(t, live, ".opencode/repo-configs/forbidden-patterns.project.js")
	if string(stagedOwned) != got {
		t.Fatalf("owned file not seeded from staging; staged!=live")
	}
}

func TestApply_FailClosedUnclassifiedPathAbortsBeforeWrite(t *testing.T) {
	live := t.TempDir()
	staging := t.TempDir()
	// An empty classifier (no rules) with the fail-closed default.
	empty := NewClassifier(ownership.EffectiveMap{}, nil)
	r := FixtureRenderer{TemplateRoot: corpusRoot}
	if err := r.Render(staging, RenderSpec{TemplateSource: "templates/core"}); err != nil {
		t.Fatalf("render: %v", err)
	}
	_, err := Apply(r, ApplyOptions{
		ProjectRoot: live, StagingDir: staging,
		Classifier: empty, HarnessVersion: "0.1.0",
		TemplateSource: "templates/core",
	})
	if err == nil {
		t.Fatalf("expected fail-closed error for unclassified paths")
	}
	// Atomicity: the live tree must be UNTOUCHED (no managed/armed/owned write,
	// no lineage.yml) because planning aborted before execution.
	entries, _ := os.ReadDir(live)
	if len(entries) != 0 {
		t.Fatalf("fail-closed abort must leave live tree empty; got %v", entries)
	}
}

func TestApply_AtomicityOwnedNeverTransientlyClobbered(t *testing.T) {
	// The design guarantees no transient clobber: render -> staging, then apply
	// writes only final values. We assert the owned file's bytes are byte-identical
	// before and after (the seam never opens it for write when present).
	live := t.TempDir()
	staging := t.TempDir()
	const sentinel = `{"sentinel":"owned-must-not-change"}`
	writeFile(t, live, ".opencode/repo-configs/forbidden-patterns.project.js", sentinel)
	writeFile(t, live, ".vh-agent-harness/AGENTS.core.md", "old managed")
	writeFile(t, live, ".vh-agent-harness/vh-harness-profile.yml", "profile: minimal\nmodules: [core]\n")

	r := FixtureRenderer{TemplateRoot: corpusRoot}
	if err := r.Render(staging, RenderSpec{TemplateSource: "templates/core"}); err != nil {
		t.Fatalf("render: %v", err)
	}
	if _, err := Apply(r, ApplyOptions{
		ProjectRoot: live, StagingDir: staging,
		Classifier: corpusClassifier(t), HarnessVersion: "0.1.0",
		TemplateSource: "templates/core",
	}); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if got := readFile(t, live, ".opencode/repo-configs/forbidden-patterns.project.js"); got != sentinel {
		t.Fatalf("owned file changed across atomic apply; before=%q after=%q", sentinel, got)
	}
}

// TestApply_DryRunWritesNothing confirms DryRun computes the full plan
// (report.Outcomes populated) but executes no write: the live tree stays empty
// and no lineage is recorded. This is the safe preview an operator/agent runs
// before applying.
func TestApply_DryRunWritesNothing(t *testing.T) {
	live := t.TempDir()
	staging := t.TempDir()

	r := FixtureRenderer{TemplateRoot: corpusRoot}
	if err := r.Render(staging, RenderSpec{
		TemplateSource: "templates/core",
		Answers:        map[string]string{"project_name": "Demo"},
	}); err != nil {
		t.Fatalf("render: %v", err)
	}

	report, err := Apply(r, ApplyOptions{
		ProjectRoot:    live,
		StagingDir:     staging,
		Classifier:     corpusClassifier(t),
		HarnessVersion: "0.1.0-test",
		TemplateSource: "templates/core",
		Answers:        map[string]string{"project_name": "Demo"},
		DryRun:         true,
	})
	if err != nil {
		t.Fatalf("Apply(dry-run): %v", err)
	}

	// Plan was computed.
	if len(report.Outcomes) == 0 {
		t.Fatal("dry-run produced no outcomes; expected the full plan")
	}
	// Lineage was NOT written.
	if report.LineagePath != "" {
		t.Errorf("dry-run set LineagePath=%q; want empty (no lineage write)", report.LineagePath)
	}
	if _, err := os.Stat(lineage.FilePath(live)); !os.IsNotExist(err) {
		t.Errorf("dry-run wrote lineage.yml; want absent (stat err=%v)", err)
	}
	// The live tree is untouched (no staged file materialized).
	entries, err := os.ReadDir(live)
	if err != nil {
		t.Fatalf("readdir live: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("dry-run wrote into the live tree; want empty, got %v", entries)
	}
}

// TestApply_LineageIdempotent confirms a no-op re-render does NOT churn
// lineage.yml: the second Apply (same answers + version → same update id) keeps
// the prior render timestamp, so the file stays byte-identical and never dirties
// git on repeated `update`.
func TestApply_LineageIdempotent(t *testing.T) {
	live := t.TempDir()
	r := FixtureRenderer{TemplateRoot: corpusRoot}
	apply := func() string {
		staging := t.TempDir()
		if err := r.Render(staging, RenderSpec{TemplateSource: "templates/core",
			Answers: map[string]string{"project_name": "Demo"}}); err != nil {
			t.Fatalf("render: %v", err)
		}
		if _, err := Apply(r, ApplyOptions{
			ProjectRoot: live, StagingDir: staging, Classifier: corpusClassifier(t),
			HarnessVersion: "0.1.0-test", TemplateSource: "templates/core",
			Answers: map[string]string{"project_name": "Demo"},
		}); err != nil {
			t.Fatalf("apply: %v", err)
		}
		b, err := os.ReadFile(lineage.FilePath(live))
		if err != nil {
			t.Fatalf("read lineage: %v", err)
		}
		return string(b)
	}
	first := apply()
	second := apply()
	if first != second {
		t.Errorf("lineage churned on no-op re-render:\n--- first ---\n%s\n--- second ---\n%s", first, second)
	}
}
