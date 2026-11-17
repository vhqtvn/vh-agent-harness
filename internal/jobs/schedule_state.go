// schedule_state.go — the scheduler's durable state file: the spec list
// plus each spec's next-run cursor, persisted by this package alone
// (path injected via SchedulerOptions.StatePath — never a session event
// type), written atomically (temp file + rename in the same directory)
// AFTER dispatch decisions so a crash between dispatch and persist
// re-dispatches on restart (at-least-once).
//
// The file is private to the scheduler dimension: the session log stays
// the single source of shared truth (job events), while schedule CADENCE
// is scheduler-owned state. Fail-closed: a corrupt or non-canonical file
// is a load error, never silently dropped schedules.
package jobs

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"time"
)

// schedulerStateVersion is the state-file schema version.
const schedulerStateVersion = 1

// schedEntry is one in-memory schedule entry: the canonical spec plus
// its next-run cursor (the next due instant, UTC).
type schedEntry struct {
	Spec    ScheduleSpec
	NextDue time.Time
}

// schedStateEntry is the JSON form of one entry.
type schedStateEntry struct {
	Spec    ScheduleSpec `json:"spec"`
	NextDue time.Time    `json:"nextDue"`
}

// schedulerStateFile is the top-level state-file shape.
type schedulerStateFile struct {
	Version int               `json:"version"`
	Specs   []schedStateEntry `json:"specs"`
}

// loadSchedulerState reads the state file at path. A missing file is a
// fresh start (nil, nil). Anything else — unreadable, malformed JSON,
// wrong version, duplicate names, non-canonical specs — is an error
// (fail-closed: never silently drop schedules).
func loadSchedulerState(path string) ([]schedEntry, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("scheduler: read state %s: %w", path, err)
	}
	var sf schedulerStateFile
	if err := json.Unmarshal(raw, &sf); err != nil {
		return nil, fmt.Errorf("scheduler: state %s is not valid JSON: %w", path, err)
	}
	if sf.Version != schedulerStateVersion {
		return nil, fmt.Errorf("scheduler: state %s has version %d, want %d", path, sf.Version, schedulerStateVersion)
	}
	seen := make(map[string]bool, len(sf.Specs))
	out := make([]schedEntry, 0, len(sf.Specs))
	for i := range sf.Specs {
		e := sf.Specs[i]
		if err := validateScheduleSpec(e.Spec); err != nil {
			return nil, fmt.Errorf("scheduler: state %s entry %d: %w", path, i, err)
		}
		if e.Spec.After != 0 {
			return nil, fmt.Errorf("scheduler: state %s entry %d (%q) is non-canonical: unresolved after", path, i, e.Spec.Name)
		}
		if e.NextDue.IsZero() {
			return nil, fmt.Errorf("scheduler: state %s entry %d (%q) has a zero cursor", path, i, e.Spec.Name)
		}
		if seen[e.Spec.Name] {
			return nil, fmt.Errorf("scheduler: state %s has duplicate schedule name %q", path, e.Spec.Name)
		}
		seen[e.Spec.Name] = true
		// Re-canonicalize defensively: storage is UTC, load stays UTC.
		e.NextDue = e.NextDue.UTC()
		out = append(out, schedEntry{Spec: e.Spec, NextDue: e.NextDue})
	}
	sortSchedEntries(out)
	return out, nil
}

// writeSchedulerState validates every entry (canonical form only: After
// resolved into At) and atomically replaces the state file: write to a
// temp file in the same directory, fsync, rename over the target. A
// crash mid-write leaves either the old or the new file, never a torn
// one; an unreferenced temp file may remain only if the crash happens
// between write and rename (harmless — the loader never reads it).
func writeSchedulerState(path string, entries []schedEntry) error {
	sf := schedulerStateFile{Version: schedulerStateVersion, Specs: make([]schedStateEntry, 0, len(entries))}
	sorted := make([]schedEntry, len(entries))
	copy(sorted, entries)
	sortSchedEntries(sorted)
	for i := range sorted {
		e := sorted[i]
		if err := validateScheduleSpec(e.Spec); err != nil {
			return fmt.Errorf("scheduler: refusing to persist entry %d: %w", i, err)
		}
		if e.Spec.After != 0 {
			return fmt.Errorf("scheduler: refusing to persist non-canonical entry %d (%q): unresolved after", i, e.Spec.Name)
		}
		if e.NextDue.IsZero() {
			return fmt.Errorf("scheduler: refusing to persist entry %d (%q): zero cursor", i, e.Spec.Name)
		}
		e.NextDue = e.NextDue.UTC()
		sf.Specs = append(sf.Specs, schedStateEntry{Spec: e.Spec, NextDue: e.NextDue})
	}
	raw, err := json.MarshalIndent(sf, "", "  ")
	if err != nil {
		return fmt.Errorf("scheduler: marshal state: %w", err)
	}
	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return fmt.Errorf("scheduler: create temp state %s: %w", tmp, err)
	}
	if _, err := f.Write(raw); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return fmt.Errorf("scheduler: write temp state %s: %w", tmp, err)
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return fmt.Errorf("scheduler: sync temp state %s: %w", tmp, err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("scheduler: close temp state %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("scheduler: rename state into place (%s → %s): %w", tmp, path, err)
	}
	return nil
}

// sortSchedEntries orders entries by (NextDue, Name): the deterministic
// dispatch priority when several schedules are due in the same gap.
func sortSchedEntries(entries []schedEntry) {
	sort.SliceStable(entries, func(i, j int) bool {
		if !entries[i].NextDue.Equal(entries[j].NextDue) {
			return entries[i].NextDue.Before(entries[j].NextDue)
		}
		return entries[i].Spec.Name < entries[j].Spec.Name
	})
}
