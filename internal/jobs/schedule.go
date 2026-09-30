// schedule.go — the schedule dimension of the jobs package: the durable
// ScheduleSpec record, its validation/UTC-canonicalization rules, and the
// PURE due-computation math (dsh schedule semantics, see
// researches/sources/deepseek-harness/session-cognition.md §schedule/).
//
// Contract (binding for the scheduler in scheduler.go):
//
//   - session-local ONLY (no global daemon): a Scheduler is bound to one
//     owner's jobs.Manager and its session log;
//   - after/at/every records are CANONICALIZED TO UTC at record time —
//     a schedule is stored as an absolute UTC instant (the first due
//     occurrence) plus a fixed-rate interval, never as a local-time or
//     tz-dependent value, so due decisions replay identically anywhere.
//     A naive local time (time.Local) is REJECTED at validation: it is
//     machine-dependent input, not a schedule;
//   - fixed-rate catch-up COLLAPSES TO THE LATEST DUE OCCURRENCE (no
//     storm replay after downtime): one dispatch per due gap, however
//     many individual occurrences were missed;
//   - dispatch is QUEUED-NOT-DELIVERED (a job/enqueued through the
//     ordinary jobs.Manager.Dispatch — never a mid-turn steer), and is
//     idle-gated (see scheduler.go);
//   - at-least-once: the durable cursor is persisted AFTER dispatch
//     decisions, so a crash between dispatch and persist re-dispatches
//     on restart (duplicates are bounded by the job layer's first-wins
//     settlement + reported-flag notice discipline, not here).
package jobs

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// ScheduleSpec is the durable record shape for one scheduled recurring or
// one-shot work item. Exactly one cadence drives it:
//
//   - one-shot: After (relative delay from record time) OR At (absolute
//     instant) — never both;
//   - recurring: Every (fixed-rate interval), optionally with a start
//     instant (At absolute, or After relative); with no start the first
//     occurrence is due at record time.
//
// The stored form is ALWAYS canonical: After is resolved against the
// record time into At, At is in UTC, and the scheduler's state file
// carries the resulting first-due cursor (see schedule_state.go).
type ScheduleSpec struct {
	// Name identifies the schedule within the session (a lowercase slug,
	// same grammar as a job kind).
	Name string `json:"name"`
	// Kind is the job kind dispatched when the schedule comes due
	// (dispatch enqueues a `<kind>-N` job carrying Payload). When empty
	// it defaults to "sched-" + Name, so schedule provenance is visible
	// in the session log's existing JobPayload.Kind string field with no
	// new event vocabulary.
	Kind string `json:"kind"`
	// After is a relative delay from record time. Canonicalized at record
	// time to an absolute UTC due instant (recordTime + After, stored in
	// At). Positive only; use At for past instants.
	After time.Duration `json:"after,omitempty"`
	// At is the absolute due instant (one-shot, or the start of an Every
	// cadence); ALWAYS canonicalized to UTC before storage. A non-UTC
	// location is a caller convenience EXCEPT time.Local, which is
	// rejected as machine-dependent. In the stored form After==0 and At
	// carries the first due instant.
	At *time.Time `json:"at,omitempty"`
	// Every is the fixed-rate interval between occurrences once due.
	// Zero means one-shot (After/At drives); positive means recurring.
	Every time.Duration `json:"every,omitempty"`
	// Payload is the work item handed to the dispatched job.
	Payload json.RawMessage `json:"payload,omitempty"`
}

// Clock is the injected timing seam (the internal/tools Clock pattern,
// extended with Now for due computation): After returns a channel that
// fires once when d elapses, Now returns the current instant in UTC.
// Tests inject a manual clock so due decisions and tick cadence are
// asserted deterministically without racing real sleeps.
type Clock interface {
	Now() time.Time
	After(d time.Duration) <-chan time.Time
}

// RealClock is the production clock backed by time.Now/time.After.
type RealClock struct{}

// Now returns the current instant in UTC.
func (RealClock) Now() time.Time { return time.Now().UTC() }

// After returns a channel firing after d.
func (RealClock) After(d time.Duration) <-chan time.Time { return time.After(d) }

// validateScheduleSpec checks the spec's shape deterministically. Errors
// are stable strings (table-tested): they name the first violated rule in
// field order (name, kind, payload, cadence, at).
func validateScheduleSpec(spec ScheduleSpec) error {
	if spec.Name == "" {
		return errors.New("scheduler: name is required")
	}
	if err := validateSlug(spec.Name, "name"); err != nil {
		return err
	}
	if spec.Kind != "" {
		if err := validateKind(spec.Kind); err != nil {
			return err
		}
	} else if err := validateSlug("sched-"+spec.Name, "derived kind"); err != nil {
		return err
	}
	if len(spec.Payload) > 0 && !json.Valid(spec.Payload) {
		return fmt.Errorf("scheduler: payload for schedule %q is not valid JSON", spec.Name)
	}
	if spec.After < 0 {
		return fmt.Errorf("scheduler: after for %q must be positive, got %s", spec.Name, spec.After)
	}
	if spec.Every < 0 {
		return fmt.Errorf("scheduler: every for %q must be positive, got %s", spec.Name, spec.Every)
	}
	if spec.At != nil {
		if spec.At.IsZero() {
			return fmt.Errorf("scheduler: at for %q is the zero time", spec.Name)
		}
		if spec.At.Location() == time.Local {
			return fmt.Errorf("scheduler: at for %q is a naive local time; pass an explicit zone or UTC (rejected: local times are machine-dependent)", spec.Name)
		}
		if spec.After > 0 {
			return fmt.Errorf("scheduler: spec %q sets both after and at; exactly one start is allowed", spec.Name)
		}
	}
	if spec.At == nil && spec.After == 0 && spec.Every == 0 {
		return fmt.Errorf("scheduler: spec %q has no cadence; set one of after, at, or every", spec.Name)
	}
	return nil
}

// validateSlug enforces the lowercase-slug grammar shared with job kinds
// (see validateKind), for names that feed derived identifiers.
func validateSlug(s, what string) error {
	if s == "" {
		return fmt.Errorf("scheduler: %s is required", what)
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z':
		case c >= '0' && c <= '9' && i > 0:
		case c == '-' && i > 0:
		default:
			return fmt.Errorf("scheduler: invalid %s %q: must be a lowercase slug [a-z][a-z0-9-]*", what, s)
		}
	}
	return nil
}

// dispatchKind returns the job kind a due spec dispatches under: the
// explicit Kind when set, else the "sched-<name>" default that keeps
// schedule provenance visible in the log's existing Kind field.
func dispatchKind(spec ScheduleSpec) string {
	if spec.Kind != "" {
		return spec.Kind
	}
	return "sched-" + spec.Name
}

// canonicalizeSpec validates the spec and resolves it against the record
// time into its durable form: After cleared, At set to the first due
// instant in UTC. The returned cursor is that first due instant (the
// value the state file seeds the next-run cursor with).
func canonicalizeSpec(spec ScheduleSpec, recordTime time.Time) (ScheduleSpec, time.Time, error) {
	if err := validateScheduleSpec(spec); err != nil {
		return ScheduleSpec{}, time.Time{}, err
	}
	due, err := firstDueUTC(spec, recordTime)
	if err != nil {
		return ScheduleSpec{}, time.Time{}, err
	}
	stored := spec
	stored.After = 0 // resolved into At below
	utc := due.UTC()
	stored.At = &utc
	return stored, utc, nil
}

// firstDueUTC computes the first due instant for a validated spec,
// purely from the record time:
//
//   - At set → At canonicalized to UTC (a past At stays past: due
//     immediately);
//   - else After > 0 → recordTime + After (UTC);
//   - else (Every alone) → recordTime (UTC): the first occurrence is
//     due at record time and catch-up collapse governs the rest.
func firstDueUTC(spec ScheduleSpec, recordTime time.Time) (time.Time, error) {
	switch {
	case spec.At != nil:
		return spec.At.UTC(), nil
	case spec.After > 0:
		return recordTime.UTC().Add(spec.After), nil
	case spec.Every > 0:
		return recordTime.UTC(), nil
	default:
		return time.Time{}, fmt.Errorf("scheduler: spec %q has no cadence", spec.Name)
	}
}

// dueAt reports whether a cursor is due at now: a cursor exactly at now
// is due (inclusive boundary), one nanosecond later is not.
func dueAt(cursor, now time.Time) bool { return !cursor.After(now) }

// advanceEvery collapses fixed-rate catch-up: given the cursor of the
// occurrence being dispatched (which may be far behind now) and a
// positive interval, it returns the NEXT cursor — the first occurrence
// strictly after now. Missed occurrences are skipped, never replayed:
// however many intervals of downtime elapsed, exactly one dispatch
// covers the gap. Precondition: every > 0 (validated upstream).
func advanceEvery(cursor, now time.Time, every time.Duration) time.Time {
	next := cursor.Add(every)
	for !next.After(now) {
		next = next.Add(every)
	}
	return next
}
