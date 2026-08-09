package managedfile

import "testing"

// TestClassifyPreserved_Matrix is the CONTRACT lock for the shared disposition
// classifier both substrate.Apply (update) and cli.doctor (lint) consume. Every
// row is one cell of the decision tree documented on ClassifyPreserved; the two
// callers MUST reach the same verdict for the same inputs, so this matrix is
// the single source of truth that keeps doctor and update in agreement on
// preserved-vs-genuine.
//
// The matrix is exhaustive over the dimensions the classifier consults:
// regenerated, hadOrigin, and the LiveState shape (absent / dir-or-stat-err /
// unreadable-regular / readable-regular with hash relations to origin+staged).
func TestClassifyPreserved_Matrix(t *testing.T) {
	const (
		origin  = "sha256:origin-v1"
		staged  = "sha256:staged-v2"
		liveOld = "sha256:origin-v1" // == origin (unedited)
		liveNew = "sha256:live-edit" // != origin, != staged (genuine edit)
		// liveSelfHeal == staged: live advanced to platform's current bytes
		// while the origin store did not (partial-failure window).
		liveSelfHeal = "sha256:staged-v2"
	)

	cases := []struct {
		name        string
		regenerated bool
		hadOrigin   bool
		origin      string
		live        LiveState
		stagedHash  string
		want        PreservedReason
	}{
		// --- bootstrap / pre-feature: no recorded origin -> never preserved ---
		{"bootstrap absent (no origin)", false, false, "", LiveState{Absent: true}, "", ""},
		{"bootstrap present diverged (no origin)", false, false, "", LiveState{IsRegular: true, Readable: true, Hash: liveNew}, staged, ""},
		{"bootstrap unreadable (no origin)", false, false, "", LiveState{IsRegular: true, Readable: false}, "", ""},

		// --- regenerated paths: NEVER preserved (always overwritten) ---
		{"regenerated absent", true, true, origin, LiveState{Absent: true}, "", ""},
		{"regenerated diverged", true, true, origin, LiveState{IsRegular: true, Readable: true, Hash: liveNew}, staged, ""},
		{"regenerated unreadable", true, true, origin, LiveState{IsRegular: true, Readable: false}, "", ""},

		// --- consumer DELETE: absent + tracked + non-regenerated ---
		{"consumer delete (tracked, non-regen)", false, true, origin, LiveState{Absent: true}, "", ConsumerDelete},

		// --- directory / stat weirdness: NOT preserved (caller reports) ---
		{"directory at path (not regular)", false, true, origin, LiveState{}, "", ""},
		{"directory at path, absent false", false, true, origin, LiveState{IsRegular: false}, "", ""},

		// --- UNREADABLE: regular file, stat ok, read fails ---
		{"unreadable regular file", false, true, origin, LiveState{IsRegular: true, Readable: false}, "", Unreadable},

		// --- readable regular file, hash relations ---
		{"unedited (live == origin)", false, true, origin, LiveState{IsRegular: true, Readable: true, Hash: liveOld}, staged, ""},
		{"genuine consumer edit (live != origin, live != staged)", false, true, origin, LiveState{IsRegular: true, Readable: true, Hash: liveNew}, staged, ConsumerEdit},
		{"partial-failure self-heal (live == staged, live != origin)", false, true, origin, LiveState{IsRegular: true, Readable: true, Hash: liveSelfHeal}, staged, ""},

		// --- empty stagedHash guard: diverged live -> ConsumerEdit (safe) ---
		{"diverged live, staged unreadable -> preserve (safe)", false, true, origin, LiveState{IsRegular: true, Readable: true, Hash: liveNew}, "", ConsumerEdit},
		// But unedited with empty stagedHash still falls through (Hash == origin).
		{"unedited live, staged unreadable -> not preserved", false, true, origin, LiveState{IsRegular: true, Readable: true, Hash: liveOld}, "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ClassifyPreserved(tc.regenerated, tc.hadOrigin, tc.origin, tc.live, tc.stagedHash)
			if got != tc.want {
				t.Fatalf("ClassifyPreserved(%v,%v,%q,%+v,%q): want %q, got %q",
					tc.regenerated, tc.hadOrigin, tc.origin, tc.live, tc.stagedHash, tc.want, got)
			}
		})
	}
}

// TestPreservedReasonValues pins the EXACT string values the taxonomy carries.
// These are the machine-readable correctness signals (a FileOutcome field
// value, and the discriminator doctor/Apply switch on). A change here is a
// behavior change that must be deliberate, not an accidental rename.
func TestPreservedReasonValues(t *testing.T) {
	want := map[PreservedReason]string{
		ConsumerEdit:   "consumer-edit",
		ConsumerDelete: "consumer-delete",
		Unreadable:     "unreadable",
	}
	for r, s := range want {
		if string(r) != s {
			t.Errorf("PreservedReason value mismatch: const %q != expected %q", r, s)
		}
	}
	// The empty/zero value is the "not preserved" sentinel and must remain "".
	var zero PreservedReason
	if zero != "" {
		t.Errorf("zero PreservedReason must be \"\" (not-preserved sentinel); got %q", zero)
	}
}
