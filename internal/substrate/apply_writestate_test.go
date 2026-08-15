package substrate

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/vhqtvn/vh-agent-harness/internal/lineage"
)

// TestWriteState_DryRun_AllNotAttempted confirms a dry-run normalizes every
// outcome's typed WriteState to WriteNotAttempted (nothing is written on a
// dry-run, including the overwrite/seed/merge routes).
func TestWriteState_DryRun_AllNotAttempted(t *testing.T) {
	live := t.TempDir()
	staging := t.TempDir()

	r := FixtureRenderer{TemplateRoot: corpusRoot}
	if err := r.Render(staging, RenderSpec{TemplateSource: "templates/core"}); err != nil {
		t.Fatalf("render: %v", err)
	}
	report, err := Apply(r, ApplyOptions{
		ProjectRoot: live, StagingDir: staging,
		Classifier: corpusClassifier(t), HarnessVersion: "0.1.0-test",
		TemplateSource: "templates/core", DryRun: true,
	})
	if err != nil {
		t.Fatalf("Apply(dry-run): %v", err)
	}
	if len(report.Outcomes) == 0 {
		t.Fatal("dry-run produced no outcomes")
	}
	for _, o := range report.Outcomes {
		if o.WriteState != WriteNotAttempted {
			t.Errorf("dry-run outcome %q: WriteState = %q, want %q", o.Path, o.WriteState, WriteNotAttempted)
		}
	}
	// A dry-run executes no write, so no write failed: the generation flag is
	// true (the field describes the EXECUTED generation; a dry-run executed
	// nothing, so nothing failed). Pinned so the flag does not silently flip.
	if !report.GenerationFullyApplied {
		t.Errorf("dry-run: GenerationFullyApplied must be true (nothing attempted, nothing failed); got false")
	}
}

// TestWriteState_LiveApply_SuccessAndNonWriteRoutes confirms the typed state on
// a successful live apply: managed-overwrite / project-seed / armed-merge routes
// are WriteSucceeded; preserved / noop / proposal / ignored routes are
// WriteNotAttempted. No outcome is left with an empty (untyped) state.
func TestWriteState_LiveApply_SuccessAndNonWriteRoutes(t *testing.T) {
	live := t.TempDir()
	staging := t.TempDir()

	// Seed a project_owned file so it routes to PRESERVED (not seeded).
	writeFile(t, live, ".opencode/repo-configs/forbidden-patterns.project.js",
		`{"sentinel":"USER owned; must be preserved"}`)
	// Seed an armed conflict so one outcome is ActionArmedProposal (no write).
	writeFile(t, live, ".vh-agent-harness/vh-harness-profile.yml",
		"profile: experimental\nmodules: [core]\n")

	r := FixtureRenderer{TemplateRoot: corpusRoot}
	if err := r.Render(staging, RenderSpec{TemplateSource: "templates/core"}); err != nil {
		t.Fatalf("render: %v", err)
	}
	report, err := Apply(r, ApplyOptions{
		ProjectRoot: live, StagingDir: staging,
		Classifier: corpusClassifier(t), HarnessVersion: "0.1.0-test",
		TemplateSource: "templates/core",
	})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	sawSucceeded, sawNotAttempted := false, false
	for _, o := range report.Outcomes {
		switch o.WriteState {
		case WriteSucceeded:
			sawSucceeded = true
		case WriteNotAttempted:
			sawNotAttempted = true
		case WriteFailed:
			t.Errorf("outcome %q: unexpected WriteFailed on a successful apply: note=%q", o.Path, o.Note)
		default:
			t.Errorf("outcome %q: WriteState left untyped (%q)", o.Path, o.WriteState)
		}
		// Non-write actions must never report WriteSucceeded.
		switch o.Action {
		case ActionProjectPreserved, ActionManagedNoop, ActionArmedNoop, ActionArmedProposal, ActionIgnoredLocal:
			if o.WriteState == WriteSucceeded {
				t.Errorf("non-write action %q reported WriteSucceeded", o.Action)
			}
		}
	}
	if !sawSucceeded {
		t.Errorf("expected at least one WriteSucceeded outcome; got %+v", report.Outcomes)
	}
	if !sawNotAttempted {
		t.Errorf("expected at least one WriteNotAttempted (preserved/proposal/noop) outcome; got %+v", report.Outcomes)
	}
}

// TestApply_LiveWriteFailure_LineageNotAdvanced is the load-bearing provenance
// lock for P1-SUBSTRATE-001 (the #1 ship-review blocker, deferred twice). A
// live-write failure is injected DETERMINISTICALLY (not via chmod): a regular
// FILE is placed at <live>/.opencode so MkdirAll for every .opencode/... dest
// fails. Apply MUST:
//   - return a nil error (a partial application is a distinct, recoverable
//     state from a hard walk/plan/lineage failure; ERROR return semantics are
//     unchanged),
//   - record WriteState=WriteFailed on the blocked routes and WriteSucceeded on
//     the unblocked routes (partial application is a REAL, observable state —
//     the writes that could land, did),
//   - report GenerationFullyApplied=false (the typed generation-level signal),
//   - NOT advance lineage: LineagePath is "" and no lineage.yml is written.
//
// This is the CRUX of the slice: lineage must never claim a generation that did
// not fully apply. v1.1 left this gap (lineage advanced on partial failure);
// this test pins the closed behavior.
func TestApply_LiveWriteFailure_LineageNotAdvanced(t *testing.T) {
	live := t.TempDir()
	staging := t.TempDir()

	// Block every write under .opencode/ deterministically: a regular file where
	// a directory is expected makes os.MkdirAll(<live>/.opencode/...) fail.
	if err := os.WriteFile(filepath.Join(live, ".opencode"), []byte("BLOCKER-not-a-directory"), 0o644); err != nil {
		t.Fatalf("plant blocker: %v", err)
	}

	r := FixtureRenderer{TemplateRoot: corpusRoot}
	if err := r.Render(staging, RenderSpec{TemplateSource: "templates/core"}); err != nil {
		t.Fatalf("render: %v", err)
	}
	report, err := Apply(r, ApplyOptions{
		ProjectRoot: live, StagingDir: staging,
		Classifier: corpusClassifier(t), HarnessVersion: "0.1.0-test",
		TemplateSource: "templates/core",
	})
	// ERROR return semantics UNCHANGED: Apply returns nil even though live
	// writes failed (partial application is not a hard error).
	if err != nil {
		t.Fatalf("Apply must return nil on live-write failure (partial application is not a hard error); got %v", err)
	}
	// Partial application is real: some routes failed, some succeeded.
	var failed, succeeded int
	for _, o := range report.Outcomes {
		switch o.WriteState {
		case WriteFailed:
			failed++
		case WriteSucceeded:
			succeeded++
		}
	}
	if failed == 0 {
		t.Errorf("expected at least one WriteFailed outcome (the blocked .opencode/* writes); got %+v", report.Outcomes)
	}
	if succeeded == 0 {
		t.Errorf("expected at least one WriteSucceeded outcome (the unblocked .vh-agent-harness/* writes); got %+v", report.Outcomes)
	}
	// Generation-level typed signal.
	if report.GenerationFullyApplied {
		t.Errorf("GenerationFullyApplied must be false when any live write failed")
	}
	// Lineage MUST NOT advance: no LineagePath, no lineage.yml on disk (first
	// install — no prior lineage existed). This is the load-bearing property.
	if report.LineagePath != "" {
		t.Errorf("LineagePath must be empty (lineage not advanced); got %q", report.LineagePath)
	}
	if _, statErr := os.Stat(lineage.FilePath(live)); statErr == nil {
		t.Errorf("lineage.yml must NOT be written when the generation did not fully apply")
	}
}

// TestApply_PartialFailure_PriorLineagePreserved is the lineage-stability crux
// for the UPDATE path: when a prior lineage record exists and a subsequent
// apply partially fails, the prior lineage MUST be preserved byte-for-byte
// (lineage did not advance for the failed generation). This proves the
// load-bearing property holds across an update, not just a first install.
func TestApply_PartialFailure_PriorLineagePreserved(t *testing.T) {
	live := t.TempDir()
	staging := t.TempDir()

	r := FixtureRenderer{TemplateRoot: corpusRoot}

	// First: a fully-applied apply establishes a lineage record.
	if err := r.Render(staging, RenderSpec{TemplateSource: "templates/core"}); err != nil {
		t.Fatalf("render (first): %v", err)
	}
	first, err := Apply(r, ApplyOptions{
		ProjectRoot: live, StagingDir: staging,
		Classifier: corpusClassifier(t), HarnessVersion: "0.1.0-test",
		TemplateSource: "templates/core",
	})
	if err != nil {
		t.Fatalf("first Apply: %v", err)
	}
	if !first.GenerationFullyApplied {
		t.Fatalf("first apply must be fully applied (no failures); got GenerationFullyApplied=false")
	}
	priorLineageBytes, err := os.ReadFile(lineage.FilePath(live))
	if err != nil {
		t.Fatalf("read prior lineage: %v", err)
	}

	// Now inject a partial-write failure for the second apply: replace a managed
	// FILE with a DIRECTORY of the same name. managedUpToDate sees the dir as
	// not-a-file -> routes to ActionManagedOverwrite; writeArmedManaged's
	// MkdirAll(dir-of-dest) succeeds but WriteFile fails (dest is a directory)
	// -> WriteFailed. Other managed files still write normally. This is a
	// genuine partial failure (some files write, one does not) that does not
	// require blocking a whole top-level dir the first apply already created.
	const blockedRel = ".opencode/agents/build.md"
	blockedPath := filepath.Join(live, filepath.FromSlash(blockedRel))
	if err := os.RemoveAll(blockedPath); err != nil {
		t.Fatalf("remove managed file to plant blocker: %v", err)
	}
	if err := os.MkdirAll(blockedPath, 0o755); err != nil {
		t.Fatalf("plant dir-at-dest: %v", err)
	}

	staging2 := t.TempDir()
	if err := r.Render(staging2, RenderSpec{TemplateSource: "templates/core"}); err != nil {
		t.Fatalf("render (second): %v", err)
	}
	second, err := Apply(r, ApplyOptions{
		ProjectRoot: live, StagingDir: staging2,
		Classifier: corpusClassifier(t), HarnessVersion: "0.1.0-test",
		TemplateSource: "templates/core",
	})
	if err != nil {
		t.Fatalf("second Apply must return nil (partial application is not a hard error); got %v", err)
	}
	if second.GenerationFullyApplied {
		t.Errorf("second apply: GenerationFullyApplied must be false (partial failure)")
	}
	if second.LineagePath != "" {
		t.Errorf("second apply: LineagePath must be empty (lineage not advanced); got %q", second.LineagePath)
	}
	// Confirm the injected failure is actually a WriteFailed (so the test is
	// exercising what it claims — a real partial failure, not a no-op).
	sawBlockedFailure := false
	for _, o := range second.Outcomes {
		if o.Path == blockedRel && o.WriteState == WriteFailed {
			sawBlockedFailure = true
		}
	}
	if !sawBlockedFailure {
		t.Errorf("expected the blocked managed file %q to report WriteFailed; the test is not exercising a real partial failure", blockedRel)
	}
	// CRUX: the prior lineage record is preserved byte-for-byte — lineage did
	// not advance for the failed generation.
	afterLineageBytes, err := os.ReadFile(lineage.FilePath(live))
	if err != nil {
		t.Fatalf("read lineage after failed apply: %v", err)
	}
	if string(afterLineageBytes) != string(priorLineageBytes) {
		t.Errorf("lineage must be byte-for-byte preserved across a partially-failed apply (lineage stability crux)")
	}
}

// TestApply_FullyApplied_GenerationFlagTrue confirms the positive case: a
// clean apply with no live-write failures reports GenerationFullyApplied=true
// and advances lineage normally. This guards against the generation flag
// accidentally flipping false on a successful apply (which would silently
// break install/update lineage recording).
func TestApply_FullyApplied_GenerationFlagTrue(t *testing.T) {
	live := t.TempDir()
	staging := t.TempDir()

	r := FixtureRenderer{TemplateRoot: corpusRoot}
	if err := r.Render(staging, RenderSpec{TemplateSource: "templates/core"}); err != nil {
		t.Fatalf("render: %v", err)
	}
	report, err := Apply(r, ApplyOptions{
		ProjectRoot: live, StagingDir: staging,
		Classifier: corpusClassifier(t), HarnessVersion: "0.1.0-test",
		TemplateSource: "templates/core",
	})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if !report.GenerationFullyApplied {
		t.Errorf("a fully-applied apply must report GenerationFullyApplied=true; got false")
	}
	if report.LineagePath != lineage.FilePath(live) {
		t.Errorf("lineage must advance on a fully-applied apply; LineagePath=%q", report.LineagePath)
	}
}

// TestWriteState_Failed_StagedReadError is a unit lock on the staged-read failure
// route of writeArmedManaged: an absent staged file yields WriteFailed + a
// human-readable Note (the typed field is the correctness signal, not the Note).
func TestWriteState_Failed_StagedReadError(t *testing.T) {
	live := t.TempDir()
	staging := t.TempDir() // intentionally no staged file
	o := FileOutcome{Path: "missing.txt", Action: ActionManagedOverwrite}
	writeArmedManaged(ApplyOptions{ProjectRoot: live, StagingDir: staging}, &o)
	if o.WriteState != WriteFailed {
		t.Fatalf("absent staged file: WriteState = %q, want %q", o.WriteState, WriteFailed)
	}
	if o.Note == "" {
		t.Errorf("WriteFailed must set a human-readable Note for diagnostics")
	}
}

// TestWriteState_Failed_ArmedMergeUnregisteredSchema is the regression lock for
// the discarded-ok bug in writeArmedManaged's armed-merge re-derive route:
// schema.SchemaForPath(rel) for a path with NO registered schema returns a
// zero-valued Schema (nil Reconciler). The old code discarded the ok bool and
// called sch.Reconciler.Reconcile(...) on the nil interface -> PANIC. The write
// path must instead fail loudly through the typed signal — WriteState=WriteFailed
// plus a human-readable Note, action downgraded to ActionArmedProposal —
// mirroring the plan-side registration validation in planArmed.
func TestWriteState_Failed_ArmedMergeUnregisteredSchema(t *testing.T) {
	live := t.TempDir()
	staging := t.TempDir()
	// A genuinely unregistered armed-looking path (no schema registry entry).
	const rel = ".vh-agent-harness/not-a-registered-armed-file.yml"
	writeFile(t, staging, rel, "key: staged\n")
	writeFile(t, live, rel, "key: project\n")
	// Force the re-derive branch: Applied carries a real merge entry, not the
	// "absent; seeded" marker that short-circuits to the staged default.
	o := FileOutcome{Path: rel, Action: ActionArmedMerged, Applied: []string{"merged a value"}}
	writeArmedManaged(ApplyOptions{ProjectRoot: live, StagingDir: staging}, &o)
	if o.WriteState != WriteFailed {
		t.Fatalf("unregistered schema on armed-merge re-derive: WriteState = %q, want %q", o.WriteState, WriteFailed)
	}
	if o.Action != ActionArmedProposal {
		t.Errorf("unregistered schema on armed-merge re-derive: Action = %q, want downgraded %q", o.Action, ActionArmedProposal)
	}
	if o.Note == "" {
		t.Errorf("WriteFailed must set a human-readable Note for diagnostics")
	}
}

// TestWriteState_Failed_WriteError is a unit lock on the live-write failure route:
// a directory at the destination path makes os.WriteFile fail, yielding
// WriteFailed. (Deterministic, not chmod.)
func TestWriteState_Failed_WriteError(t *testing.T) {
	live := t.TempDir()
	staging := t.TempDir()
	writeFile(t, staging, "managed.txt", "content\n")
	// Destination exists as a DIRECTORY -> os.WriteFile fails with "is a directory".
	if err := os.MkdirAll(filepath.Join(live, "managed.txt"), 0o755); err != nil {
		t.Fatalf("plant dir-at-dest: %v", err)
	}
	o := FileOutcome{Path: "managed.txt", Action: ActionManagedOverwrite}
	writeArmedManaged(ApplyOptions{ProjectRoot: live, StagingDir: staging}, &o)
	if o.WriteState != WriteFailed {
		t.Fatalf("write error (dest is dir): WriteState = %q, want %q", o.WriteState, WriteFailed)
	}
	if o.Note == "" {
		t.Errorf("WriteFailed must set a human-readable Note for diagnostics")
	}
}
