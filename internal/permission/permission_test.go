package permission

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

// TestAction_String verifies the canonical action names used in messages.
func TestAction_String(t *testing.T) {
	cases := map[Action]string{
		Allow: "allow",
		Deny:  "deny",
		Ask:   "ask",
	}
	for a, want := range cases {
		if got := a.String(); got != want {
			t.Errorf("%d.String() = %q, want %q", a, got, want)
		}
	}
}

// TestNoOpHook_AllowAndWarn verifies the slice-4a contract: NoOpHook ALWAYS
// returns Allow with the documented stub reason, AND prints the loud noop
// warning so the safety gap is never silent.
func TestNoOpHook_AllowAndWarn(t *testing.T) {
	// Capture the warning via the package stderr seam.
	var buf bytes.Buffer
	saved := stderr
	stderr = &buf
	defer func() { stderr = saved }()

	a, reason, err := NoOpHook{}.Evaluate(context.Background(), []string{"echo", "hello"})
	if err != nil {
		t.Fatalf("NoOpHook returned error: %v", err)
	}
	if a != Allow {
		t.Errorf("action = %s, want Allow", a)
	}
	if reason != noopReason {
		t.Errorf("reason = %q, want %q", reason, noopReason)
	}
	out := buf.String()
	for _, want := range []string{"WARNING", "slice-4a stub", "no command checking is active", "Slice 4b"} {
		if !strings.Contains(out, want) {
			t.Errorf("warning missing %q; got:\n%s", want, out)
		}
	}
}
