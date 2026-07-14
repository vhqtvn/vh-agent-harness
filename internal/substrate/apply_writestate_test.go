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

// TestApply_LiveWriteFailure_ReturnsNilError_SetsWriteFailed is the
// apply-before-persist ordering + return-semantics lock for P1-LINEAGE-002 v1.1.
// A live write failure is injected DETERMINISTICALLY (not via chmod): a regular
// FILE is placed at <live>/.opencode so MkdirAll for every .opencode/... dest
// fails. Apply MUST still return a nil error (return semantics UNCHANGED in this
// slice — lifting a live-write failure into an Apply error is P1-SUBSTRATE-001),
// the failed routes MUST carry WriteState=WriteFailed, the successful routes
// (under .vh-agent-harness/, whose dir is not blocked) MUST carry
// WriteState=WriteSucceeded, and lineage.yml MUST still be written (the gap
// P1-SUBSTRATE-001 will close).
func TestApply_LiveWriteFailure_ReturnsNilError_SetsWriteFailed(t *testing.T) {
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
	// Return semantics UNCHANGED: Apply returns nil even though live writes failed.
	if err != nil {
		t.Fatalf("Apply must return nil on live-write failure in v1.1 (return semantics unchanged); got %v", err)
	}
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
	// Lineage MUST still be written — this is the gap P1-SUBSTRATE-001 will close
	// (lineage must not advance for a generation that did not fully apply).
	if report.LineagePath == "" {
		t.Errorf("expected lineage.yml to still be written (v1.1 does NOT gate lineage — that is P1-SUBSTRATE-001)")
	}
	if _, statErr := os.Stat(lineage.FilePath(live)); statErr != nil {
		t.Errorf("lineage.yml not written: %v", statErr)
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
