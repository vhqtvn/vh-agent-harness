package substrate

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	corpus "github.com/vhqtvn/vh-agent-harness" // root package: embed + CoreOwnershipDefaults
	"github.com/vhqtvn/vh-agent-harness/internal/lineage"
	"github.com/vhqtvn/vh-agent-harness/internal/managedfile"
	"github.com/vhqtvn/vh-agent-harness/internal/originhash"
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

// TestApply_ManagedNoopWhenByteIdentical confirms a platform_managed file whose
// live instance is byte-identical to the freshly rendered corpus is reported as
// ActionManagedNoop (managed-unchanged) and NOT rewritten — distinguishing a
// no-op refresh from real churn. A drifted live copy still overwrites.
func TestApply_ManagedNoopWhenByteIdentical(t *testing.T) {
	live := t.TempDir()
	staging := t.TempDir()

	r := FixtureRenderer{TemplateRoot: corpusRoot}
	if err := r.Render(staging, RenderSpec{TemplateSource: "templates/core"}); err != nil {
		t.Fatalf("render: %v", err)
	}

	// Seed the live tree with the EXACT staged corpus bytes for one managed
	// file, and DRIFTED bytes for another. We must read staged bytes after
	// render so they match byte-for-byte (no hardcoded copy that drifts).
	const upToDateRel = ".vh-agent-harness/AGENTS.core.md"
	const driftedRel = ".opencode/agents/build.md" // platform_managed in the corpus
	stagedUpToDate, _ := os.ReadFile(filepath.Join(staging, upToDateRel))
	writeFile(t, live, upToDateRel, string(stagedUpToDate))
	writeFile(t, live, driftedRel, "DRIFTED CONTENT not the corpus\n")

	report, err := Apply(r, ApplyOptions{
		ProjectRoot: live, StagingDir: staging,
		Classifier:     corpusClassifier(t),
		HarnessVersion: "0.1.0-test", TemplateSource: "templates/core",
	})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}

	byPath := map[string]FileOutcome{}
	for _, o := range report.Outcomes {
		byPath[o.Path] = o
	}

	// Byte-identical -> managed-unchanged (no write).
	if got, ok := byPath[upToDateRel]; !ok || got.Action != ActionManagedNoop {
		t.Fatalf("byte-identical managed file: want %s, got %+v", ActionManagedNoop, got)
	}

	// Drifted -> managed-overwrite (still written).
	if got, ok := byPath[driftedRel]; !ok || got.Action != ActionManagedOverwrite {
		t.Fatalf("drifted managed file: want %s, got %+v", ActionManagedOverwrite, got)
	}
}

// TestApply_ManagedNoopIsPureSkip verifies a managed-unchanged outcome touches
// neither the file bytes nor the mtime: the noop must be a pure skip (no write
// at all). This guards executeOutcome's switch against accidentally writing
// managed-noop files.
func TestApply_ManagedNoopIsPureSkip(t *testing.T) {
	live := t.TempDir()
	staging := t.TempDir()

	r := FixtureRenderer{TemplateRoot: corpusRoot}
	if err := r.Render(staging, RenderSpec{TemplateSource: "templates/core"}); err != nil {
		t.Fatalf("render: %v", err)
	}

	const rel = ".vh-agent-harness/AGENTS.core.md"
	stagedBytes, _ := os.ReadFile(filepath.Join(staging, rel))
	writeFile(t, live, rel, string(stagedBytes))

	livePath := filepath.Join(live, rel)
	beforeInfo, err := os.Stat(livePath)
	if err != nil {
		t.Fatalf("stat before: %v", err)
	}

	if _, err := Apply(r, ApplyOptions{
		ProjectRoot: live, StagingDir: staging,
		Classifier:     corpusClassifier(t),
		HarnessVersion: "0.1.0-test", TemplateSource: "templates/core",
	}); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	afterInfo, err := os.Stat(livePath)
	if err != nil {
		t.Fatalf("stat after: %v", err)
	}
	// A pure skip must preserve BOTH the size and the mtime (a write would
	// bump the mtime even when the bytes are identical).
	if afterInfo.Size() != beforeInfo.Size() {
		t.Errorf("managed-noop rewrote the file: size changed %d -> %d", beforeInfo.Size(), afterInfo.Size())
	}
	if !afterInfo.ModTime().Equal(beforeInfo.ModTime()) {
		t.Errorf("managed-noop must not bump mtime: before=%v after=%v", beforeInfo.ModTime(), afterInfo.ModTime())
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

// TestApply_ManagedAbsentRoutesToOverwriteNotNoop locks the safety fallback for
// the FIRST-INSTALL / missing-file path: a platform_managed file whose live
// instance is ABSENT must route to ActionManagedOverwrite, never silently no-op
// as ActionManagedNoop. Byte-identical → noop is covered by
// TestApply_ManagedNoopWhenByteIdentical; this test guards the absent branch so
// an unreadable/missing live managed file can never silently skip.
func TestApply_ManagedAbsentRoutesToOverwriteNotNoop(t *testing.T) {
	live := t.TempDir() // empty: no managed file present (first-install shape)
	staging := t.TempDir()

	r := FixtureRenderer{TemplateRoot: corpusRoot}
	if err := r.Render(staging, RenderSpec{TemplateSource: "templates/core"}); err != nil {
		t.Fatalf("render: %v", err)
	}

	report, err := Apply(r, ApplyOptions{
		ProjectRoot: live, StagingDir: staging,
		Classifier:     corpusClassifier(t),
		HarnessVersion: "0.1.0-test", TemplateSource: "templates/core",
	})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}

	const rel = ".vh-agent-harness/AGENTS.core.md" // platform_managed in the corpus
	byPath := map[string]FileOutcome{}
	for _, o := range report.Outcomes {
		byPath[o.Path] = o
	}
	got, ok := byPath[rel]
	if !ok {
		t.Fatalf("absent managed file produced no outcome; got %+v", report.Outcomes)
	}
	if got.Action != ActionManagedOverwrite {
		t.Fatalf("absent managed file: want %s (never noop), got %s", ActionManagedOverwrite, got.Action)
	}
}

// TestManagedUpToDate_AbsentOrUnreadableFallsBackToFalse locks the read-error /
// unreadable fallback of managedUpToDate directly. managedUpToDate is the unit
// that embodies the "absent or unreadable live managed file must fall back to
// overwrite, never noop" contract. The unreadable case is awkward to trigger
// through the black-box Apply path: a directory at the live path fails the
// subsequent executeOutcome write, and a chmod-000 file is still readable as
// root. So the fallback is locked at the unit that owns it. A directory livePath
// exists (so it is not the absent case) yet is not a readable file, modeling the
// unreadable path; both must return false so planOutcome routes to overwrite.
func TestManagedUpToDate_AbsentOrUnreadableFallsBackToFalse(t *testing.T) {
	staging := t.TempDir()
	const stagedRel = "staged.txt"
	writeFile(t, staging, stagedRel, "staged corpus bytes\n")
	stagedPath := filepath.Join(staging, stagedRel)

	// Absent live file (first-install / missing-file path) -> not up to date.
	absentLive := filepath.Join(t.TempDir(), "does-not-exist")
	if managedUpToDate(stagedPath, absentLive) {
		t.Errorf("absent live file: managedUpToDate must be false (route to overwrite), got true")
	}

	// Unreadable live "file": the live path resolves to a DIRECTORY. It exists,
	// so it is not the absent case, but it is not a readable managed file — the
	// safe default is to treat it as not-up-to-date and overwrite.
	dirLive := t.TempDir() // a directory is not a readable managed file
	if managedUpToDate(stagedPath, dirLive) {
		t.Errorf("unreadable (directory) live path: managedUpToDate must be false (route to overwrite), got true")
	}
}

// TestApply_OriginHashThreeWayPreservesConsumerEdits is the BEHAVIORAL-CLOSURE
// crux for the origin-hash update sync (decision memo
// origin-hash-update-sync.md, OPT-A, porting hermes skills_sync's three-way
// mechanism). It exercises the REAL apply path twice against the same live tree
// with the REAL ownership classifier and the REAL origin-hash sidecar
// (.vh-agent-harness/origin-hashes.json) — no mocks:
//
//  1. Apply #1 (install): seeds platform_managed files and RECORDS their origin
//     hashes.
//  2. Between applies, hand-edit ONE platform_managed file (simulating a
//     consumer edit like vh-video-maker's rule 6 in the composed AGENTS.md) and
//     leave another unedited; produce a new staging with genuinely different
//     platform bytes for both (simulating a new harness release).
//  3. Apply #2 (update): the EDITED file must be PRESERVED
//     (ActionManagedDiverged, consumer bytes byte-identical, NOT clobbered);
//     the UNEDITED file must be UPDATED normally to the new platform bytes
//     (ActionManagedOverwrite). The edited file's origin hash must be carried
//     forward unchanged (the platform did not write it).
//
// This is the hermes _is_tracked_user_modification test ported to the harness
// apply path, proven end-to-end against real machinery.
func TestApply_OriginHashThreeWayPreservesConsumerEdits(t *testing.T) {
	live := t.TempDir()
	r := FixtureRenderer{TemplateRoot: corpusRoot}

	const editedRel = ".vh-agent-harness/AGENTS.core.md" // platform_managed in the corpus
	const uneditedRel = ".opencode/agents/build.md"      // platform_managed in the corpus

	// --- Apply #1: render + apply (install). Seeds managed files, records
	// origin hashes. ---
	staging := t.TempDir()
	if err := r.Render(staging, RenderSpec{TemplateSource: "templates/core"}); err != nil {
		t.Fatalf("render #1: %v", err)
	}
	if _, err := Apply(r, ApplyOptions{
		ProjectRoot: live, StagingDir: staging,
		Classifier: corpusClassifier(t), HarnessVersion: "0.1.0-originhash",
		TemplateSource: "templates/core",
	}); err != nil {
		t.Fatalf("Apply #1: %v", err)
	}

	// Origin-hash store MUST have been written, with an entry for every
	// platform_managed file the platform seeded.
	store, err := originhash.Read(live)
	if err != nil {
		t.Fatalf("read origin-hash store after Apply #1: %v", err)
	}
	if store == nil {
		t.Fatalf("origin-hash store was not written by Apply #1")
	}
	for _, rel := range []string{editedRel, uneditedRel} {
		if _, ok := store.Lookup(rel); !ok {
			t.Fatalf("origin-hash store missing platform_managed entry for %q", rel)
		}
	}

	// --- Between applies: hand-edit one file; simulate a new platform release
	// (different bytes) for BOTH files. ---
	const consumerEdit = "CONSUMER HAND-EDIT — must survive update (origin-hash three-way)\n"
	writeFile(t, live, editedRel, consumerEdit)
	// uneditedRel is left exactly as Apply #1 wrote it.

	staging2 := t.TempDir()
	if err := r.Render(staging2, RenderSpec{TemplateSource: "templates/core"}); err != nil {
		t.Fatalf("render #2: %v", err)
	}
	// Mutate the staged copies so a real platform change exists for both files
	// (proves the unedited file is actually updated, not skipped as byte-identical).
	appendMarkerToStaged := func(rel, marker string) {
		p := filepath.Join(staging2, rel)
		b, rerr := os.ReadFile(p)
		if rerr != nil {
			t.Fatalf("read staged %s: %v", rel, rerr)
		}
		b = append(b, []byte("\n# platform release marker: "+marker+"\n")...)
		if werr := os.WriteFile(p, b, 0o644); werr != nil {
			t.Fatalf("mutate staged %s: %v", rel, werr)
		}
	}
	appendMarkerToStaged(editedRel, "edited-release")
	appendMarkerToStaged(uneditedRel, "unedited-release")

	// --- Apply #2: update. ---
	report2, err := Apply(r, ApplyOptions{
		ProjectRoot: live, StagingDir: staging2,
		Classifier: corpusClassifier(t), HarnessVersion: "0.1.0-originhash",
		TemplateSource: "templates/core",
	})
	if err != nil {
		t.Fatalf("Apply #2: %v", err)
	}
	byPath := map[string]FileOutcome{}
	for _, o := range report2.Outcomes {
		byPath[o.Path] = o
	}

	// EDITED file: diverged → preserved (NOT clobbered).
	ed, ok := byPath[editedRel]
	if !ok {
		t.Fatalf("edited file produced no outcome; got %+v", report2.Outcomes)
	}
	if ed.Action != ActionManagedDiverged {
		t.Fatalf("edited platform_managed file: want %s, got %s (note=%q)", ActionManagedDiverged, ed.Action, ed.Note)
	}
	if gotEdited := readFile(t, live, editedRel); gotEdited != consumerEdit {
		t.Fatalf("consumer edit was NOT preserved (clobbered); want=%q got=%q", consumerEdit, gotEdited)
	}

	// UNEDITED file: updated to new platform bytes (overwrite, content changed).
	un, ok := byPath[uneditedRel]
	if !ok {
		t.Fatalf("unedited file produced no outcome; got %+v", report2.Outcomes)
	}
	if un.Action != ActionManagedOverwrite {
		t.Fatalf("unedited platform_managed file: want %s (content changed), got %s (note=%q)", ActionManagedOverwrite, un.Action, un.Note)
	}
	stagedUnedited, _ := os.ReadFile(filepath.Join(staging2, uneditedRel))
	if gotUnedited := readFile(t, live, uneditedRel); string(stagedUnedited) != gotUnedited {
		t.Fatalf("unedited file was not updated to new platform bytes")
	}

	// Origin-hash store after Apply #2: edited file retains the ORIGINAL origin
	// (the platform version from Apply #1 — it did not write this generation);
	// unedited file's origin advanced to the new platform bytes.
	store2, err := originhash.Read(live)
	if err != nil {
		t.Fatalf("read origin-hash store after Apply #2: %v", err)
	}
	if store2 == nil {
		t.Fatalf("origin-hash store not written by Apply #2")
	}
	if origEdited, ok := store2.Lookup(editedRel); !ok || origEdited != store.OriginHashes[editedRel] {
		t.Fatalf("edited file origin hash must be carried forward unchanged across a skip; got %q", origEdited)
	}
	if origUnedited, ok := store2.Lookup(uneditedRel); !ok {
		t.Fatalf("unedited file origin hash missing after Apply #2")
	} else {
		// Unedited was overwritten with new bytes → origin must be the new staged hash.
		wantUnedited := originhash.Digest(stagedUnedited)
		if origUnedited != wantUnedited {
			t.Fatalf("unedited file origin hash not advanced to new platform bytes; want %q got %q", wantUnedited, origUnedited)
		}
	}
}

// TestApply_OriginHashRespectsConsumerDeletedManagedFile locks the deletion-
// suppression half of the origin-hash update sync: a platform_managed file the
// consumer DELETED (but the platform previously rendered, so it has a recorded
// origin hash) must NOT be re-seeded on the next update. Mirrors hermes
// skills_sync :914-916.
func TestApply_OriginHashRespectsConsumerDeletedManagedFile(t *testing.T) {
	live := t.TempDir()
	r := FixtureRenderer{TemplateRoot: corpusRoot}

	const rel = ".vh-agent-harness/AGENTS.core.md"

	// Apply #1: install (seeds the file, records origin hash).
	staging := t.TempDir()
	if err := r.Render(staging, RenderSpec{TemplateSource: "templates/core"}); err != nil {
		t.Fatalf("render #1: %v", err)
	}
	if _, err := Apply(r, ApplyOptions{
		ProjectRoot: live, StagingDir: staging,
		Classifier: corpusClassifier(t), HarnessVersion: "0.1.0-originhash",
		TemplateSource: "templates/core",
	}); err != nil {
		t.Fatalf("Apply #1: %v", err)
	}

	// Consumer deletes the managed file.
	if err := os.Remove(filepath.Join(live, rel)); err != nil {
		t.Fatalf("remove managed file: %v", err)
	}

	// Apply #2: must NOT re-seed the deleted file (origin hash recorded → respected).
	staging2 := t.TempDir()
	if err := r.Render(staging2, RenderSpec{TemplateSource: "templates/core"}); err != nil {
		t.Fatalf("render #2: %v", err)
	}
	report, err := Apply(r, ApplyOptions{
		ProjectRoot: live, StagingDir: staging2,
		Classifier: corpusClassifier(t), HarnessVersion: "0.1.0-originhash",
		TemplateSource: "templates/core",
	})
	if err != nil {
		t.Fatalf("Apply #2: %v", err)
	}
	byPath := map[string]FileOutcome{}
	for _, o := range report.Outcomes {
		byPath[o.Path] = o
	}
	got, ok := byPath[rel]
	if !ok {
		t.Fatalf("deleted managed file produced no outcome; got %+v", report.Outcomes)
	}
	if got.Action != ActionManagedDiverged {
		t.Fatalf("deleted managed file: want %s (not re-seeded), got %s (note=%q)", ActionManagedDiverged, got.Action, got.Note)
	}
	// The file must NOT have been re-created on disk.
	if _, statErr := os.Stat(filepath.Join(live, rel)); !os.IsNotExist(statErr) {
		t.Fatalf("deleted managed file was re-seeded on update (must be respected); stat err=%v", statErr)
	}
}

// TestApply_OriginHashBootstrapNoPriorOriginSeedsNormally locks the bootstrap
// case: with no prior origin-hash store (first install, or a pre-origin-hash
// install), platform_managed files are seeded/overwritten normally (NOT skipped
// as "diverged"), and origin hashes are then recorded for the next apply. This
// guards against the three-way check accidentally treating "no prior origin" as
// divergence (which would block first install).
func TestApply_OriginHashBootstrapNoPriorOriginSeedsNormally(t *testing.T) {
	live := t.TempDir() // empty: no prior origin-hash store
	staging := t.TempDir()
	r := FixtureRenderer{TemplateRoot: corpusRoot}
	if err := r.Render(staging, RenderSpec{TemplateSource: "templates/core"}); err != nil {
		t.Fatalf("render: %v", err)
	}
	report, err := Apply(r, ApplyOptions{
		ProjectRoot: live, StagingDir: staging,
		Classifier: corpusClassifier(t), HarnessVersion: "0.1.0-originhash",
		TemplateSource: "templates/core",
	})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	// A platform_managed file on first install must be seeded (overwrite), never
	// diverged (no prior origin → bootstrap).
	const rel = ".vh-agent-harness/AGENTS.core.md"
	byPath := map[string]FileOutcome{}
	for _, o := range report.Outcomes {
		byPath[o.Path] = o
	}
	got, ok := byPath[rel]
	if !ok {
		t.Fatalf("managed file produced no outcome; got %+v", report.Outcomes)
	}
	if got.Action != ActionManagedOverwrite {
		t.Fatalf("bootstrap first-install managed file: want %s, got %s (must seed, not skip)", ActionManagedOverwrite, got.Action)
	}
	// Origin hashes recorded for the next apply.
	store, err := originhash.Read(live)
	if err != nil {
		t.Fatalf("read origin-hash store after bootstrap apply: %v", err)
	}
	if store == nil {
		t.Fatalf("origin-hash store not written after bootstrap apply")
	}
	if _, ok := store.Lookup(rel); !ok {
		t.Fatalf("origin-hash store missing bootstrap entry for %q", rel)
	}
}

// TestApply_OriginHashUpstreamRemovedDeManifestsOnly locks the
// upstream-removed case: a platform_managed path present in the prior origin
// store but NOT in the current generation's staging is dropped from the store
// (de-manifested) while its on-disk copy is left untouched. (The path not being
// staged means planOutcome never processes it, so it never reaches an outcome;
// the new store is built only from this generation's outcomes, so the prior
// entry simply disappears.)
func TestApply_OriginHashUpstreamRemovedDeManifestsOnly(t *testing.T) {
	live := t.TempDir()
	// Seed a prior origin store with a path that will NOT be staged this apply.
	prior := originhash.New()
	const ghostRel = ".opencode/agents/ghost-removed-by-upstream.md"
	prior.OriginHashes[ghostRel] = originhash.Digest([]byte("old platform bytes"))
	// Plant the on-disk copy the consumer still has.
	writeFile(t, live, ghostRel, "consumer still has this file on disk\n")
	if err := prior.Write(live); err != nil {
		t.Fatalf("seed prior store: %v", err)
	}

	// Render the real corpus (which does NOT contain ghostRel) and apply.
	staging := t.TempDir()
	r := FixtureRenderer{TemplateRoot: corpusRoot}
	if err := r.Render(staging, RenderSpec{TemplateSource: "templates/core"}); err != nil {
		t.Fatalf("render: %v", err)
	}
	if _, err := Apply(r, ApplyOptions{
		ProjectRoot: live, StagingDir: staging,
		Classifier: corpusClassifier(t), HarnessVersion: "0.1.0-originhash",
		TemplateSource: "templates/core",
	}); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	// The ghost path is de-manifested: it is NOT in the new store.
	store, err := originhash.Read(live)
	if err != nil {
		t.Fatalf("read origin-hash store after apply: %v", err)
	}
	if store == nil {
		t.Fatalf("origin-hash store not written")
	}
	if _, ok := store.Lookup(ghostRel); ok {
		t.Fatalf("upstream-removed path %q must be de-manifested (dropped from store); still present", ghostRel)
	}
	// But its on-disk copy is left untouched (NEVER deleted).
	if got := readFile(t, live, ghostRel); got != "consumer still has this file on disk\n" {
		t.Fatalf("upstream-removed on-disk copy was altered/deleted; got %q", got)
	}
}

// TestApply_OriginHashCorruptStoreBlocksUpdate locks the fail-closed contract
// for the origin-hash store: a PRESENT-BUT-CORRUPT or unsupported-schema
// sidecar makes originhash.Read return an error, and Apply MUST surface that
// error (aborting before any live-tree write) rather than treating it as a
// no-origin bootstrap. A nil store would make Lookup report no prior origin for
// every platform_managed path, skipping the three-way divergence check for ALL
// such files and wholesale-overwriting every consumer hand-edit — exactly the
// silent-clobber data loss this feature exists to prevent (and the store would
// then be rewritten fresh, erasing the prior origins). Fail-closed here is the
// package's documented contract ("a corrupted file is never silently trusted").
func TestApply_OriginHashCorruptStoreBlocksUpdate(t *testing.T) {
	const editedRel = ".vh-agent-harness/AGENTS.core.md" // platform_managed in the corpus
	const consumerEdit = "CONSUMER HAND-EDIT — a corrupt store must NOT let this be clobbered\n"

	cases := []struct {
		name        string
		storeBytes  []byte // raw bytes planted at the origin-hash sidecar path
		errFragment string // substring expected in the Apply error
	}{
		{
			name:        "malformed-json",
			storeBytes:  []byte("{ this is not valid json "),
			errFragment: "originhash",
		},
		{
			name:        "unsupported-schema-version",
			storeBytes:  []byte(`{"schema_version":"999","origin_hashes":{}}` + "\n"),
			errFragment: "unsupported",
		},
		{
			// Schema-version-valid but origin_hashes is explicitly null. This
			// binary's Write never produces this form (it always marshals a
			// non-nil map as "{}"), so null means hand-edited/foreign/truncated.
			// A nil map treated as an empty bootstrap store would make Lookup
			// report no prior origin for every path → every consumer edit clobbered.
			name:        "schema-valid-null-hashes",
			storeBytes:  []byte(`{"schema_version":"1","origin_hashes":null}` + "\n"),
			errFragment: "missing or null origin_hashes",
		},
		{
			// Same defect via an absent field.
			name:        "schema-valid-absent-hashes",
			storeBytes:  []byte(`{"schema_version":"1"}` + "\n"),
			errFragment: "missing or null origin_hashes",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			live := t.TempDir()

			// Plant a corrupt origin-hash store so Read returns an error.
			dir := filepath.Join(live, originhash.DirName)
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatalf("mkdir store dir: %v", err)
			}
			if err := os.WriteFile(filepath.Join(dir, originhash.FileName), tc.storeBytes, 0o644); err != nil {
				t.Fatalf("plant corrupt store: %v", err)
			}

			// Plant a consumer-edited managed file on the live tree. Under the
			// OLD swallow-the-error behavior this would be wholesale-overwritten
			// (the bug); under fail-closed it MUST survive because Apply aborts
			// before any write.
			writeFile(t, live, editedRel, consumerEdit)

			staging := t.TempDir()
			r := FixtureRenderer{TemplateRoot: corpusRoot}
			if err := r.Render(staging, RenderSpec{TemplateSource: "templates/core"}); err != nil {
				t.Fatalf("render: %v", err)
			}

			_, err := Apply(r, ApplyOptions{
				ProjectRoot: live, StagingDir: staging,
				Classifier: corpusClassifier(t), HarnessVersion: "0.1.0-originhash",
				TemplateSource: "templates/core",
			})
			if err == nil {
				t.Fatalf("Apply must FAIL on a corrupt origin-hash store (fail-closed); it succeeded — consumer edits would be silently clobbered")
			}
			if !strings.Contains(err.Error(), tc.errFragment) {
				t.Fatalf("Apply error must mention %q; got %q", tc.errFragment, err.Error())
			}

			// The consumer edit survives: Apply aborted before any live-tree
			// write (the plan phase loaded the store and failed).
			if got := readFile(t, live, editedRel); got != consumerEdit {
				t.Fatalf("consumer edit was clobbered despite a corrupt store; want=%q got=%q", consumerEdit, got)
			}
		})
	}
}

// TestApply_OriginHashPartialFailureSelfHealsOnRetry locks the recovery path for
// the partial-failure window (commit-review b-F2): if a prior apply's live-tree
// writes landed but its origin-store write did NOT (e.g. .vh-agent-harness/
// transiently non-writable), the store is left at the OLD origin while the live
// tree holds the platform's NEW bytes. A naive three-way check would see
// live(new) != origin(old) and misclassify the file as ActionManagedDiverged
// (consumer-edited) — permanently skipping platform updates to a file the
// consumer never touched. The staged-comparison self-heal routes this to
// ActionManagedNoop (live already equals what the platform would write) and
// advances the origin, so the interrupted generation recovers on retry.
func TestApply_OriginHashPartialFailureSelfHealsOnRetry(t *testing.T) {
	live := t.TempDir()
	r := FixtureRenderer{TemplateRoot: corpusRoot}
	const rel = ".vh-agent-harness/AGENTS.core.md" // platform_managed in the corpus

	// --- Apply #1: install. live gets v1 (corpus), store records v1 origin. ---
	staging := t.TempDir()
	if err := r.Render(staging, RenderSpec{TemplateSource: "templates/core"}); err != nil {
		t.Fatalf("render #1: %v", err)
	}
	if _, err := Apply(r, ApplyOptions{
		ProjectRoot: live, StagingDir: staging,
		Classifier: corpusClassifier(t), HarnessVersion: "0.1.0-originhash",
		TemplateSource: "templates/core",
	}); err != nil {
		t.Fatalf("Apply #1: %v", err)
	}
	store1, err := originhash.Read(live)
	if err != nil || store1 == nil {
		t.Fatalf("store not written by Apply #1: %v", err)
	}
	origV1, ok := store1.Lookup(rel)
	if !ok {
		t.Fatalf("store missing origin for %q after Apply #1", rel)
	}

	// --- Simulate the partial-failure state: the platform wrote v2 to the live
	// tree but the origin-store write failed, so the store is still at v1. ---
	const v2Marker = "\n# platform release v2 (partial-failure: live advanced, store did not)\n"
	// v2 = the platform's new bytes (v1 corpus content + a release marker).
	v1Live := readFile(t, live, rel)
	v2Bytes := append([]byte(v1Live), []byte(v2Marker)...)
	// Write v2 to the live tree WITHOUT going through Apply (which would advance
	// the store). The store stays at v1 origin — the interrupted-generation state.
	writeFile(t, live, rel, string(v2Bytes))
	// Confirm the store was NOT advanced (still v1) — i.e. the simulated state
	// really is "live=v2, store=v1-origin".
	if s, _ := originhash.Read(live); s == nil || s.OriginHashes[rel] != origV1 {
		t.Fatalf("test setup invariant: store origin changed to %q, expected still v1 %q", s.OriginHashes[rel], origV1)
	}

	// --- Apply #2: retry with staging = v2 (the same bytes now on live). ---
	staging2 := t.TempDir()
	if err := r.Render(staging2, RenderSpec{TemplateSource: "templates/core"}); err != nil {
		t.Fatalf("render #2: %v", err)
	}
	// Make staged match the v2 bytes already on live (the platform's current release).
	stagedRel2 := filepath.Join(staging2, rel)
	stagedBase, serr := os.ReadFile(stagedRel2)
	if serr != nil {
		t.Fatalf("read staged %s: %v", stagedRel2, serr)
	}
	if werr := os.WriteFile(stagedRel2, append(stagedBase, []byte(v2Marker)...), 0o644); werr != nil {
		t.Fatalf("mutate staged %s: %v", stagedRel2, werr)
	}
	stagedV2, _ := os.ReadFile(stagedRel2)
	wantV2 := originhash.Digest(stagedV2)

	report, err := Apply(r, ApplyOptions{
		ProjectRoot: live, StagingDir: staging2,
		Classifier: corpusClassifier(t), HarnessVersion: "0.1.0-originhash",
		TemplateSource: "templates/core",
	})
	if err != nil {
		t.Fatalf("Apply #2: %v", err)
	}
	var got FileOutcome
	for _, o := range report.Outcomes {
		if o.Path == rel {
			got = o
		}
	}
	// Self-heal: live already equals the platform's current bytes → noop (NOT
	// diverged). A naive three-way check would return diverged here and skip the
	// file permanently despite no consumer edit.
	if got.Action != ActionManagedNoop {
		t.Fatalf("partial-failure self-heal: want %s for live==staged, got %s (note=%q); the file would be permanently skipped despite no consumer edit",
			ActionManagedNoop, got.Action, got.Note)
	}
	// Origin advanced to v2 (the bytes now confirmed on both live and staging),
	// so the interrupted generation is reconciled — it did NOT stay at v1.
	store2, err := originhash.Read(live)
	if err != nil || store2 == nil {
		t.Fatalf("store not written by Apply #2: %v", err)
	}
	if got2, ok := store2.Lookup(rel); !ok || got2 != wantV2 {
		t.Fatalf("origin must advance to v2 after self-heal; want %q got %q (ok=%v)", wantV2, got2, ok)
	}
}

// TestApply_OriginHashUnreadableLiveFileIsPreserved is the SUPPORTED-PLATFORM
// integration check for the fail-closed path on an authored managed file whose
// bytes cannot be read by the update process (commit-review b-F1, round 2):
// os.Stat succeeds but the live read fails (write-permitted-but-not-readable,
// mode 0200). We CANNOT confirm whether the consumer edited it, so the safe
// choice — the feature's core guarantee — is to PRESERVE (ActionManagedDiverged),
// NOT fall through to overwrite, which would silently clobber a possible edit.
//
// This test relies on OS permission bits to enforce the read failure and SKIPS
// under root / permissive filesystems (where reads succeed regardless of mode).
// The DETERMINISTIC coverage of the same safety path — proving read-failure is
// distinct from absent and unedited WITHOUT depending on OS perms — lives in
// TestApply_OriginHashUnreadableLiveFile_Deterministic, which injects a known
// read failure through the readLiveFile seam. Keep both: this one proves the
// real-filesystem path works on platforms that honor the bits; the deterministic
// one guarantees the behavior is observed in every CI environment.
func TestApply_OriginHashUnreadableLiveFileIsPreserved(t *testing.T) {
	live := t.TempDir()
	r := FixtureRenderer{TemplateRoot: corpusRoot}
	const rel = ".vh-agent-harness/AGENTS.core.md" // authored platform_managed

	// --- Apply #1: install. Records an origin hash so the three-way check runs. ---
	staging := t.TempDir()
	if err := r.Render(staging, RenderSpec{TemplateSource: "templates/core"}); err != nil {
		t.Fatalf("render #1: %v", err)
	}
	if _, err := Apply(r, ApplyOptions{
		ProjectRoot: live, StagingDir: staging,
		Classifier: corpusClassifier(t), HarnessVersion: "0.1.0-originhash",
		TemplateSource: "templates/core",
	}); err != nil {
		t.Fatalf("Apply #1: %v", err)
	}

	// Snapshot the content, then make the live file write-but-not-read (mode
	// 0200: owner write, no read). os.Stat still succeeds; os.ReadFile fails.
	origContent := readFile(t, live, rel)
	livePath := filepath.Join(live, rel)
	if err := os.Chmod(livePath, 0o200); err != nil {
		t.Fatalf("chmod 0200: %v", err)
	}
	// Self-check: if reads still succeed (root, or a permissive filesystem), the
	// write-but-not-read scenario is not enforceable here — skip, do not pass.
	if _, rerr := os.ReadFile(livePath); rerr == nil {
		t.Skip("cannot enforce write-but-not-read permissions on this platform/filesystem (reads succeed)")
	}

	// --- Apply #2: re-render the same content. The live file is unreadable. ---
	staging2 := t.TempDir()
	if err := r.Render(staging2, RenderSpec{TemplateSource: "templates/core"}); err != nil {
		t.Fatalf("render #2: %v", err)
	}
	report, err := Apply(r, ApplyOptions{
		ProjectRoot: live, StagingDir: staging2,
		Classifier: corpusClassifier(t), HarnessVersion: "0.1.0-originhash",
		TemplateSource: "templates/core",
	})
	if err != nil {
		t.Fatalf("Apply #2: %v", err)
	}
	var got FileOutcome
	for _, o := range report.Outcomes {
		if o.Path == rel {
			got = o
		}
	}
	// Unreadable live file with a prior origin → PRESERVE (diverged), NOT
	// overwrite. We cannot confirm the file is unedited, so we never clobber it.
	if got.Action != ActionManagedDiverged {
		t.Fatalf("unreadable live managed file: want %s (preserved, never clobber), got %s (note=%q)",
			ActionManagedDiverged, got.Action, got.Note)
	}

	// Restore read perm and confirm the platform did NOT overwrite the file: the
	// original content survived byte-for-byte.
	if err := os.Chmod(livePath, 0o644); err != nil {
		t.Fatalf("chmod restore 0644: %v", err)
	}
	if got := readFile(t, live, rel); got != origContent {
		t.Fatalf("unreadable live file was overwritten (clobbered); content changed from %q to %q", origContent, got)
	}
}

// TestApply_OriginHashUnreadableLiveFile_Deterministic is the DETERMINISTIC
// behavioral-closure crux for the unreadable-live-file safety path (F8). It
// injects a KNOWN read failure through the readLiveFile seam — NO reliance on
// OS permission bits (which the OS-permission integration test above cannot
// enforce under root / permissive filesystems). The deterministic test proves,
// in every CI environment:
//
//  1. read-failure ≠ absent ≠ unedited: a live file that EXISTS (stat ok,
//     regular) but whose read FAILS routes to the TYPED PreservedReason ==
//     managedfile.Unreadable — distinct from ConsumerDelete (absent) and from
//     the empty/unedited disposition that falls through to overwrite/noop.
//  2. no overwrite: the consumer's bytes are preserved byte-for-byte.
//  3. fail-closed/preserved: Action == ActionManagedDiverged (NEVER clobber a
//     possible edit the read could not inspect).
//  4. origin not advanced: the diverged outcome carries the prior origin
//     forward, so the next apply still detects the divergence.
//  5. report names path+reason: the Note identifies the path as unreadable.
//
// The seam (package-scoped readLiveFile) is restored to os.ReadFile on cleanup
// so no other test is affected.
func TestApply_OriginHashUnreadableLiveFile_Deterministic(t *testing.T) {
	live := t.TempDir()
	r := FixtureRenderer{TemplateRoot: corpusRoot}
	const rel = ".vh-agent-harness/AGENTS.core.md" // authored platform_managed

	// --- Apply #1: install. Records an origin hash so the three-way check runs. ---
	staging := t.TempDir()
	if err := r.Render(staging, RenderSpec{TemplateSource: "templates/core"}); err != nil {
		t.Fatalf("render #1: %v", err)
	}
	if _, err := Apply(r, ApplyOptions{
		ProjectRoot: live, StagingDir: staging,
		Classifier: corpusClassifier(t), HarnessVersion: "0.1.0-originhash",
		TemplateSource: "templates/core",
	}); err != nil {
		t.Fatalf("Apply #1: %v", err)
	}
	store1, err := originhash.Read(live)
	if err != nil || store1 == nil {
		t.Fatalf("store not written by Apply #1: %v", err)
	}
	origOrigin, ok := store1.Lookup(rel)
	if !ok {
		t.Fatalf("store missing origin for %q after Apply #1", rel)
	}
	origContent := readFile(t, live, rel)

	// --- Inject a DETERMINISTIC read failure via the readLiveFile seam. The
	// live file genuinely EXISTS on disk (stat succeeds, regular file) — only
	// the read is forced to fail, exactly the write-permitted-but-not-readable
	// condition the OS-perm test can only enforce under restrictive perms. ---
	prevReadLiveFile := readLiveFile
	readLiveFile = func(p string) ([]byte, error) {
		return nil, errors.New("injected deterministic read failure (F8 seam)")
	}
	t.Cleanup(func() { readLiveFile = prevReadLiveFile })

	// --- Apply #2: the live file exists but is unreadable. ---
	staging2 := t.TempDir()
	if err := r.Render(staging2, RenderSpec{TemplateSource: "templates/core"}); err != nil {
		t.Fatalf("render #2: %v", err)
	}
	// Mutate the staged copy so a real platform change exists — proves the
	// unreadable file is NOT being treated as a byte-identical noop.
	stagedRel2 := filepath.Join(staging2, rel)
	stagedBase, serr := os.ReadFile(stagedRel2)
	if serr != nil {
		t.Fatalf("read staged %s: %v", rel, serr)
	}
	if werr := os.WriteFile(stagedRel2, append(stagedBase, []byte("\n# platform release marker (unreadable test)\n")...), 0o644); werr != nil {
		t.Fatalf("mutate staged %s: %v", rel, werr)
	}

	report, err := Apply(r, ApplyOptions{
		ProjectRoot: live, StagingDir: staging2,
		Classifier: corpusClassifier(t), HarnessVersion: "0.1.0-originhash",
		TemplateSource: "templates/core",
	})
	if err != nil {
		t.Fatalf("Apply #2: %v", err)
	}
	var got FileOutcome
	for _, o := range report.Outcomes {
		if o.Path == rel {
			got = o
		}
	}

	// (3) fail-closed/preserved: Action == ActionManagedDiverged (NEVER overwrite).
	if got.Action != ActionManagedDiverged {
		t.Fatalf("unreadable live managed file: want %s (preserved, never clobber), got %s (note=%q)",
			ActionManagedDiverged, got.Action, got.Note)
	}

	// (1) read-failure ≠ absent ≠ unedited: the TYPED PreservedReason is exactly
	// managedfile.Unreadable — NOT ConsumerDelete (absent) and NOT the empty
	// disposition of an unedited file (which would have fallen through to
	// overwrite/noop). This is the typed correctness signal the F8 seam exists
	// to make assertable without OS-perm dependence.
	if got.PreservedReason != managedfile.Unreadable {
		t.Fatalf("unreadable live file: want typed PreservedReason %q, got %q (note=%q) — read-failure must be distinct from absent (ConsumerDelete) and unedited (empty)",
			managedfile.Unreadable, got.PreservedReason, got.Note)
	}

	// (5) report names path+reason: the Note identifies the file as unreadable.
	if !strings.Contains(got.Note, "unreadable") {
		t.Errorf("Note should name the unreadable reason; got %q", got.Note)
	}

	// (2) no overwrite: the consumer's original bytes survived byte-for-byte.
	// Read directly via os.ReadFile (NOT the seam — the seam only governs the
	// three-way live read inside planOutcome).
	if gotContent := readFile(t, live, rel); gotContent != origContent {
		t.Fatalf("unreadable live file was overwritten (clobbered); content changed from %q to %q", origContent, gotContent)
	}

	// (4) origin not advanced: the diverged outcome carries the prior origin
	// forward, so the next apply still detects the (still-unreadable) divergence.
	store2, err := originhash.Read(live)
	if err != nil || store2 == nil {
		t.Fatalf("store not written by Apply #2: %v", err)
	}
	if got2, ok2 := store2.Lookup(rel); !ok2 || got2 != origOrigin {
		t.Fatalf("origin must carry forward unchanged across an unreadable skip; want %q (ok=%v), got %q (ok=%v)",
			origOrigin, ok, got2, ok2)
	}
}
