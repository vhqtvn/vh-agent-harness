package jobs

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestSchedulerStateRoundtripAtomicReplace: persisting writes the full
// state via temp+rename — after writes the directory holds exactly the
// one state file (no leftover temp), and a reload reproduces the
// canonical entries.
func TestSchedulerStateRoundtripAtomicReplace(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	t0 := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	at := t0.Add(time.Hour)
	entries := []schedEntry{
		{Spec: ScheduleSpec{Name: "beta", Every: time.Minute, At: &at}, NextDue: t0.Add(2 * time.Minute)},
		{Spec: ScheduleSpec{Name: "alpha", Every: time.Minute, At: &at}, NextDue: t0.Add(time.Minute)},
	}
	if err := writeSchedulerState(path, entries); err != nil {
		t.Fatalf("writeSchedulerState: %v", err)
	}
	assertSingleFile(t, dir, "state.json")

	loaded, err := loadSchedulerState(path)
	if err != nil {
		t.Fatalf("loadSchedulerState: %v", err)
	}
	if len(loaded) != 2 {
		t.Fatalf("loaded %d entries, want 2", len(loaded))
	}
	// Load re-sorts by (NextDue, Name).
	if loaded[0].Spec.Name != "alpha" || loaded[1].Spec.Name != "beta" {
		t.Fatalf("load order = %q,%q — want alpha,beta (NextDue,Name order)", loaded[0].Spec.Name, loaded[1].Spec.Name)
	}
	for _, e := range loaded {
		if e.NextDue.Location() != time.UTC {
			t.Fatalf("loaded NextDue location = %v, want UTC", e.NextDue.Location())
		}
		if e.Spec.After != 0 {
			t.Fatalf("loaded spec %q keeps After=%s — state must be canonical", e.Spec.Name, e.Spec.After)
		}
	}

	// Rewrite replaces atomically: still exactly one file, new content.
	entries[0].NextDue = t0.Add(9 * time.Minute)
	if err := writeSchedulerState(path, entries); err != nil {
		t.Fatalf("rewrite: %v", err)
	}
	assertSingleFile(t, dir, "state.json")
	loaded, err = loadSchedulerState(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if !loaded[1].NextDue.Equal(t0.Add(9 * time.Minute)) {
		t.Fatalf("rewrite did not land: %+v", loaded)
	}
}

func assertSingleFile(t *testing.T, dir, want string) {
	t.Helper()
	ents, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(ents) != 1 || ents[0].Name() != want {
		names := make([]string, 0, len(ents))
		for _, e := range ents {
			names = append(names, e.Name())
		}
		t.Fatalf("state dir holds %v, want exactly [%s] (no temp leftovers)", names, want)
	}
}

// TestSchedulerStateMissingFileIsFresh: a missing state file is a fresh
// start, not an error.
func TestSchedulerStateMissingFileIsFresh(t *testing.T) {
	loaded, err := loadSchedulerState(filepath.Join(t.TempDir(), "absent.json"))
	if err != nil {
		t.Fatalf("missing state must be fresh, got %v", err)
	}
	if len(loaded) != 0 {
		t.Fatalf("fresh state has entries: %+v", loaded)
	}
}

// TestSchedulerStateMalformedRejected: a corrupt or non-canonical state
// file fails closed at load.
func TestSchedulerStateMalformedRejected(t *testing.T) {
	dir := t.TempDir()

	garbage := filepath.Join(dir, "garbage.json")
	if err := os.WriteFile(garbage, []byte("{not json"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, err := loadSchedulerState(garbage); err == nil {
		t.Fatal("garbage state accepted")
	}

	// Non-canonical stored spec (unresolved After) is rejected: state is
	// written only by canonicalizeSpec output.
	nonCanon := filepath.Join(dir, "noncanon.json")
	at := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	bad := []schedEntry{{Spec: ScheduleSpec{Name: "x", After: time.Minute}, NextDue: at}}
	if err := writeSchedulerState(nonCanon, bad); err == nil {
		t.Fatal("writeSchedulerState accepted a non-canonical spec")
	}
}
