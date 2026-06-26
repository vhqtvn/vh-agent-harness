package runshape

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeRunShape writes a run-shape.yml under root/.vh-agent-harness/ with the
// given raw lifecycle YAML body (appended under `lifecycle:`).
func writeRunShape(t *testing.T, root, lifecycleBody string) {
	t.Helper()
	dir := filepath.Join(root, DirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", DirName, err)
	}
	body := "run_shape_version: \"0.1\"\nlifecycle:\n" + lifecycleBody
	if err := os.WriteFile(filepath.Join(dir, FileName), []byte(body), 0o644); err != nil {
		t.Fatalf("write run-shape: %v", err)
	}
}

// TestLoad_ValidPointers — several scripts/ pointers load cleanly.
func TestLoad_ValidPointers(t *testing.T) {
	root := t.TempDir()
	writeRunShape(t, root, strings.Join([]string{
		"  pre_up: scripts/clean.sh",
		"  post_up: scripts/migrate-db.sh",
		"  pre_exec: scripts/setup.sh",
		"  post_exec: scripts/teardown.sh",
		"  pre_down: ",
		"  post_down: ",
	}, "\n"))
	rs, err := LoadForRoot(root)
	if err != nil {
		t.Fatalf("LoadForRoot: %v", err)
	}
	if got := rs.Lifecycle[HookPreUp]; got != "scripts/clean.sh" {
		t.Errorf("pre_up = %q, want scripts/clean.sh", got)
	}
	if got := rs.Lifecycle[HookPostUp]; got != "scripts/migrate-db.sh" {
		t.Errorf("post_up = %q, want scripts/migrate-db.sh", got)
	}
	if _, ok := rs.Lifecycle[HookPreDown]; ok {
		t.Errorf("empty pre_down should be absent (no-op), not stored")
	}
}

// TestLoadForRoot_AbsentIsNoop — no run-shape file => zero RunShape, no error.
// This is the invariant that preserves Slices 1–4: a repo with no run-shape sees
// zero hook activity.
func TestLoadForRoot_AbsentIsNoop(t *testing.T) {
	root := t.TempDir()
	rs, err := LoadForRoot(root)
	if err != nil {
		t.Fatalf("absent run-shape must not error; got %v", err)
	}
	if rs == nil || len(rs.Lifecycle) != 0 {
		t.Fatalf("absent run-shape must yield empty Lifecycle; got %+v", rs)
	}
}

// TestLoad_InlineShellRejected — a value with shell metachars is rejected with a
// typed InlineShellError. This is the explicit "no inline shell in the schema".
func TestLoad_InlineShellRejected(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value string
	}{
		{"semicolon", "echo hi; rm -rf /"},
		{"pipe", "cat x | grep y"},
		{"ampersand", "sleep 1 & echo done"},
		{"backtick", "x=`whoami`"},
		{"cmd-subst", "x=$(whoami)"},
		{"redirect", "echo x > /etc/passwd"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			writeRunShape(t, root, "  pre_up: \""+tc.value+"\"")
			_, err := LoadForRoot(root)
			if err == nil {
				t.Fatalf("inline-shell value %q should be rejected", tc.value)
			}
			var ise *InlineShellError
			if !errors.As(err, &ise) {
				t.Errorf("expected *InlineShellError, got %T: %v", err, err)
			}
		})
	}
}

// TestLoad_NonPathCommandRejected — a space-separated command with NO metachars
// (e.g. "rm -rf /") is still rejected because it does not resolve under scripts/.
func TestLoad_NonPathCommandRejected(t *testing.T) {
	root := t.TempDir()
	writeRunShape(t, root, "  pre_up: rm -rf /")
	_, err := LoadForRoot(root)
	if err == nil {
		t.Fatalf("\"rm -rf /\" should be rejected")
	}
	var npp *NotAPathPointerError
	if !errors.As(err, &npp) {
		t.Errorf("expected *NotAPathPointerError, got %T: %v", err, err)
	}
}

// TestLoad_AbsolutePathRejected — absolute paths are rejected.
func TestLoad_AbsolutePathRejected(t *testing.T) {
	root := t.TempDir()
	writeRunShape(t, root, "  pre_up: /bin/evil.sh")
	_, err := LoadForRoot(root)
	var npp *NotAPathPointerError
	if !errors.As(err, &npp) {
		t.Fatalf("absolute path should yield *NotAPathPointerError; got %T: %v", err, err)
	}
	if !strings.Contains(npp.Reason, "absolute") {
		t.Errorf("reason should mention absolute; got %q", npp.Reason)
	}
}

// TestLoad_TraversalRejected — "../" escape is rejected.
func TestLoad_TraversalRejected(t *testing.T) {
	root := t.TempDir()
	writeRunShape(t, root, "  pre_up: scripts/../../etc/passwd")
	_, err := LoadForRoot(root)
	var npp *NotAPathPointerError
	if !errors.As(err, &npp) {
		t.Fatalf("traversal should yield *NotAPathPointerError; got %T: %v", err, err)
	}
}

// TestLoad_OutsideScriptsRejected — a relative path NOT under scripts/ is rejected.
func TestLoad_OutsideScriptsRejected(t *testing.T) {
	root := t.TempDir()
	writeRunShape(t, root, "  pre_up: hooks/other.sh")
	_, err := LoadForRoot(root)
	var npp *NotAPathPointerError
	if !errors.As(err, &npp) {
		t.Fatalf("non-scripts path should yield *NotAPathPointerError; got %T: %v", err, err)
	}
}

// TestLoad_UnknownLifecycleKeyRejected — a typo'd key is rejected, NOT silently
// executed. This is the "fixed lifecycle points only" guarantee.
func TestLoad_UnknownLifecycleKeyRejected(t *testing.T) {
	root := t.TempDir()
	writeRunShape(t, root, "  pre_upp: scripts/clean.sh")
	_, err := LoadForRoot(root)
	if err == nil {
		t.Fatalf("unknown lifecycle key should be rejected")
	}
	var uke *UnknownLifecycleHookError
	if !errors.As(err, &uke) {
		t.Errorf("expected *UnknownLifecycleHookError, got %T: %v", err, err)
	}
	if !strings.Contains(uke.Key, "pre_upp") {
		t.Errorf("error should name the bad key; got %q", uke.Key)
	}
}

// TestLoad_MalformedYAMLRejected — garbage YAML is a typed error, not a crash.
func TestLoad_MalformedYAMLRejected(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, DirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	garbage := []byte("lifecycle: [this is not: a: valid: map")
	if err := os.WriteFile(filepath.Join(dir, FileName), garbage, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	_, err := LoadForRoot(root)
	var mrs *MalformedRunShapeError
	if !errors.As(err, &mrs) {
		t.Errorf("expected *MalformedRunShapeError, got %T: %v", err, err)
	}
}

// TestIsKnown — the fixed set is exactly the documented nine points.
func TestIsKnown_FixedSet(t *testing.T) {
	want := []LifecycleHook{
		HookOnFirstInstall, HookOnUpdate,
		HookPreUp, HookPostUp, HookPreDown, HookPostDown,
		HookPreExec, HookPostExec, HookOnUninstall,
	}
	if len(KnownHooks()) != len(want) {
		t.Fatalf("KnownHooks len = %d, want %d", len(KnownHooks()), len(want))
	}
	for _, h := range want {
		if !IsKnown(h) {
			t.Errorf("%q should be known", h)
		}
	}
	for _, bad := range []LifecycleHook{"pre_upp", "on_install", "", "POST_UP"} {
		if IsKnown(bad) {
			t.Errorf("%q should NOT be known", bad)
		}
	}
}
