package jobs

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// TestScheduleCanonicalizesToUTC implements the slice-4 skipped-TODO
// contract: schedule records are canonicalized to UTC at record time so
// due decisions replay identically in any location, and the dispatched
// kind keeps schedule provenance in the existing JobPayload string
// fields (queued-not-delivered dispatch itself is proven against the
// real Manager in scheduler_test.go).
func TestScheduleCanonicalizesToUTC(t *testing.T) {
	local := time.FixedZone("caller-zone", 8*3600)
	at := time.Date(2026, 8, 20, 9, 0, 0, 0, local) // 01:00 UTC

	recordTime := time.Date(2026, 8, 20, 0, 0, 0, 0, time.UTC)
	spec := ScheduleSpec{Name: "nightly", Kind: "digest", At: &at, Every: 24 * time.Hour, Payload: json.RawMessage(`{}`)}

	stored, cursor, err := canonicalizeSpec(spec, recordTime)
	if err != nil {
		t.Fatalf("canonicalizeSpec: %v", err)
	}
	if got := cursor.Location(); got != time.UTC {
		t.Fatalf("first due cursor location = %v, want UTC", got)
	}
	if want := at.In(time.UTC); !cursor.Equal(want) {
		t.Fatalf("cursor = %v, want %v", cursor, want)
	}
	if stored.At == nil || stored.At.Location() != time.UTC {
		t.Fatalf("stored At = %v, want the UTC first-due instant", stored.At)
	}
	if !stored.At.Equal(at.In(time.UTC)) {
		t.Fatalf("stored At = %v, want %v", stored.At, at.In(time.UTC))
	}
	if stored.After != 0 {
		t.Fatalf("stored After = %s, want 0 (resolved into At at record time)", stored.After)
	}
	if stored.Every != 24*time.Hour {
		t.Fatalf("stored Every = %s, want 24h", stored.Every)
	}
}

// TestCanonicalizeAfterResolvesAgainstRecordTime: a relative After is
// resolved into an absolute UTC At at record time — the stored record
// never depends on a later wall clock.
func TestCanonicalizeAfterResolvesAgainstRecordTime(t *testing.T) {
	recordTime := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	spec := ScheduleSpec{Name: "once", After: 90 * time.Minute}

	stored, cursor, err := canonicalizeSpec(spec, recordTime)
	if err != nil {
		t.Fatalf("canonicalizeSpec: %v", err)
	}
	want := recordTime.Add(90 * time.Minute)
	if !cursor.Equal(want) || stored.At == nil || !stored.At.Equal(want) {
		t.Fatalf("cursor/stored At = %v/%v, want %v", cursor, stored.At, want)
	}
	if stored.After != 0 {
		t.Fatalf("stored After = %s, want 0", stored.After)
	}
}

// TestDispatchKindDefault: an empty Kind derives "sched-<name>" so
// provenance rides the existing JobPayload.Kind field.
func TestDispatchKindDefault(t *testing.T) {
	if got, want := dispatchKind(ScheduleSpec{Name: "nightly", At: ptrTime(time.Now().UTC())}), "sched-nightly"; got != want {
		t.Fatalf("dispatchKind = %q, want %q", got, want)
	}
	if got, want := dispatchKind(ScheduleSpec{Name: "nightly", Kind: "digest"}), "digest"; got != want {
		t.Fatalf("dispatchKind = %q, want %q", got, want)
	}
}

func ptrTime(t time.Time) *time.Time { return &t }

// TestValidateScheduleSpec: deterministic validation errors — naive
// local At rejected, ambiguous start rejected, missing cadence rejected,
// slug grammar enforced, invalid payload rejected.
func TestValidateScheduleSpec(t *testing.T) {
	utc := time.Date(2026, 8, 20, 1, 0, 0, 0, time.UTC)
	fz := time.Date(2026, 8, 20, 9, 0, 0, 0, time.FixedZone("caller-zone", 8*3600))
	naive := time.Date(2026, 8, 20, 9, 0, 0, 0, time.Local)
	zero := time.Time{}

	cases := []struct {
		name    string
		spec    ScheduleSpec
		wantSub string
	}{
		{"naive local at", ScheduleSpec{Name: "n", At: &naive}, "naive local time"},
		{"zero at", ScheduleSpec{Name: "n", At: &zero}, "zero time"},
		{"after and at together", ScheduleSpec{Name: "n", After: time.Minute, At: &utc}, "both after and at"},
		{"no cadence", ScheduleSpec{Name: "n"}, "no cadence"},
		{"negative after", ScheduleSpec{Name: "n", After: -time.Second}, "must be positive"},
		{"negative every", ScheduleSpec{Name: "n", Every: -time.Hour}, "must be positive"},
		{"empty name", ScheduleSpec{At: &utc}, "name is required"},
		{"bad name", ScheduleSpec{Name: "Nightly", At: &utc}, "invalid name"},
		{"bad kind", ScheduleSpec{Name: "n", Kind: "Digest", At: &utc}, "invalid kind"},
		{"invalid payload", ScheduleSpec{Name: "n", At: &utc, Payload: json.RawMessage(`{`)}, "not valid JSON"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateScheduleSpec(tc.spec)
			if err == nil {
				t.Fatalf("validateScheduleSpec accepted invalid spec %+v", tc.spec)
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Fatalf("error = %q, want substring %q", err.Error(), tc.wantSub)
			}
		})
	}

	// Accepted shapes: explicit zones canonicalize; every alone; every
	// with a start; one-shots.
	ok := []ScheduleSpec{
		{Name: "zoned", At: &fz, Every: time.Hour},
		{Name: "every-alone", Every: time.Minute},
		{Name: "after-one-shot", After: time.Second},
		{Name: "at-one-shot", At: &utc},
		{Name: "default-kind", Every: time.Minute},
	}
	for i := range ok {
		if err := validateScheduleSpec(ok[i]); err != nil {
			t.Fatalf("validateScheduleSpec rejected valid spec %+v: %v", ok[i], err)
		}
	}
}

// TestFirstDueUTC: the pure first-due table — past At stays due
// immediately; Every alone starts at record time.
func TestFirstDueUTC(t *testing.T) {
	record := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	past := time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC)
	future := time.Date(2026, 8, 21, 12, 0, 0, 0, time.UTC)

	cases := []struct {
		name string
		spec ScheduleSpec
		want time.Time
	}{
		{"at past", ScheduleSpec{At: &past}, past},
		{"at future", ScheduleSpec{At: &future}, future},
		{"after", ScheduleSpec{After: 5 * time.Minute}, record.Add(5 * time.Minute)},
		{"every alone", ScheduleSpec{Every: time.Hour}, record},
		{"every with at start", ScheduleSpec{Every: time.Hour, At: &future}, future},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := firstDueUTC(tc.spec, record)
			if err != nil {
				t.Fatalf("firstDueUTC: %v", err)
			}
			if !got.Equal(tc.want) {
				t.Fatalf("firstDueUTC = %v, want %v", got, tc.want)
			}
			if got.Location() != time.UTC {
				t.Fatalf("firstDueUTC location = %v, want UTC", got.Location())
			}
		})
	}
}

// TestDueAtBoundary: the due boundary is inclusive — a cursor exactly at
// now is due; one nanosecond in the future is not.
func TestDueAtBoundary(t *testing.T) {
	now := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	if !dueAt(now, now) {
		t.Fatal("cursor == now must be due")
	}
	if dueAt(now.Add(time.Nanosecond), now) {
		t.Fatal("cursor one nanosecond after now must not be due")
	}
	if !dueAt(now.Add(-time.Hour), now) {
		t.Fatal("past cursor must be due")
	}
}

// TestAdvanceEveryCollapse: the fixed-rate catch-up collapse table —
// after dispatching the occurrence at cursor, the next cursor is the
// first occurrence STRICTLY after now, skipping every missed one.
func TestAdvanceEveryCollapse(t *testing.T) {
	t0 := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	e := time.Minute

	cases := []struct {
		name string
		now  time.Time
		want time.Time
	}{
		{"no catch-up", t0, t0.Add(e)},
		{"just before second", t0.Add(e - time.Nanosecond), t0.Add(e)},
		{"exactly second", t0.Add(e), t0.Add(2 * e)},
		{"3 intervals missed", t0.Add(3 * e), t0.Add(4 * e)},
		{"2.5 intervals missed", t0.Add(2*e + 30*time.Second), t0.Add(3 * e)},
		{"10 intervals missed", t0.Add(10 * e), t0.Add(11 * e)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := advanceEvery(t0, tc.now, e)
			if !got.Equal(tc.want) {
				t.Fatalf("advanceEvery = %v, want %v", got, tc.want)
			}
			if !got.After(tc.now) {
				t.Fatalf("advanced cursor %v must be strictly after now %v", got, tc.now)
			}
		})
	}
}
