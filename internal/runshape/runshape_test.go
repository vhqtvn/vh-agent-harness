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

// writeRawRunShape writes arbitrary raw YAML as the run-shape file (used for
// LoadMinMode tests that need exec_sandbox + unrelated blocks).
func writeRawRunShape(t *testing.T, root, raw string) {
	t.Helper()
	dir := filepath.Join(root, DirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", DirName, err)
	}
	if err := os.WriteFile(filepath.Join(dir, FileName), []byte(raw), 0o644); err != nil {
		t.Fatalf("write run-shape: %v", err)
	}
}

// TestLoadMinMode locks the floor reader contract: tolerant of unrelated
// blocks (decodes ONLY exec_sandbox) but FAIL-CLOSED on a deliberate-but-broken
// floor (present-but-wrong-type min_mode, document syntax error).
func TestLoadMinMode(t *testing.T) {
	// present + strict
	root := t.TempDir()
	writeRawRunShape(t, root, "runtime: {backend: host-shell}\nexec_sandbox:\n  min_mode: strict\n")
	if got, err := LoadMinMode(root); err != nil || got != "strict" {
		t.Fatalf("strict floor: got %q err=%v, want strict/nil", got, err)
	}

	// absent block => empty (no floor)
	root2 := t.TempDir()
	writeRawRunShape(t, root2, "runtime: {backend: host-shell}\n")
	if got, err := LoadMinMode(root2); err != nil || got != "" {
		t.Fatalf("absent exec_sandbox: got %q err=%v, want empty/nil", got, err)
	}

	// absent FILE => empty (no floor), no error
	root3 := t.TempDir()
	if got, err := LoadMinMode(root3); err != nil || got != "" {
		t.Fatalf("absent file: got %q err=%v, want empty/nil", got, err)
	}

	// robust to a malformed lifecycle block: the floor still reads. This is the
	// core isolation property — LoadMinMode does NOT depend on Load's strict
	// lifecycle validation, so an unrelated run-shape VALUE issue cannot drop
	// the floor.
	root4 := t.TempDir()
	writeRawRunShape(t, root4, "lifecycle:\n  hooks:\n    totally_bogus_hook: scripts/x.sh\nexec_sandbox:\n  min_mode: strict\n")
	if got, err := LoadMinMode(root4); err != nil || got != "strict" {
		t.Fatalf("floor must survive unrelated lifecycle malformation: got %q err=%v, want strict/nil", got, err)
	}

	// FAIL-CLOSED: min_mode KEY absent inside a present block => error (a present
	// exec_sandbox block REQUIRES min_mode; an operator who writes the block
	// intended a floor). This closes the key-typo hole (a misspelled key leaves
	// min_mode absent). `exec_sandbox: {}` and `exec_sandbox:` (null) differ:
	// null decodes to nil (block absent => no floor); an empty/explicit map is a
	// present block without min_mode => fail-closed.
	root5a := t.TempDir()
	writeRawRunShape(t, root5a, "exec_sandbox: {}\n")
	if _, err := LoadMinMode(root5a); err == nil {
		t.Fatalf("exec_sandbox {} (present block, no min_mode): expected fail-closed error, got nil")
	}

	// FAIL-CLOSED: misspelled min_mode KEY (the key-typo hole) => error.
	root5b := t.TempDir()
	writeRawRunShape(t, root5b, "exec_sandbox:\n  min_mdoe: strict\n")
	if _, err := LoadMinMode(root5b); err == nil {
		t.Fatalf("misspelled min_mode key (min_mdoe): expected fail-closed error, got nil")
	}

	// forward-compat: unknown FUTURE key alongside a present min_mode is OK
	// (min_mode still resolves). This preserves compatibility with newer
	// run-shapes carrying additional exec_sandbox keys.
	root5c := t.TempDir()
	writeRawRunShape(t, root5c, "exec_sandbox:\n  min_mode: strict\n  future_key: x\n")
	if got, err := LoadMinMode(root5c); err != nil || got != "strict" {
		t.Fatalf("forward-compat unknown key + present min_mode: got %q err=%v, want strict/nil", got, err)
	}

	// FAIL-CLOSED: present-but-wrong-TYPE min_mode must error (not silently
	// return ""/off). A typo like `min_mode: [strict]` must not disable a floor
	// the operator deliberately set. This is the B1 contract from the
	// exec-sandbox commit-review.
	for _, bad := range []struct {
		name string
		raw  string
	}{
		{"sequence", "exec_sandbox:\n  min_mode: [strict]\n"},
		{"int", "exec_sandbox:\n  min_mode: 2\n"},
		{"map", "exec_sandbox:\n  min_mode: {x: y}\n"},
		{"bool", "exec_sandbox:\n  min_mode: true\n"},
	} {
		rb := t.TempDir()
		writeRawRunShape(t, rb, bad.raw)
		got, err := LoadMinMode(rb)
		if err == nil {
			t.Fatalf("wrong-type min_mode (%s): expected fail-closed error, got nil (value=%q)", bad.name, got)
		}
		if got != "" {
			t.Fatalf("wrong-type min_mode (%s): expected empty value on error, got %q", bad.name, got)
		}
	}

	// FAIL-CLOSED: document-level YAML syntax error => error (not silent "").
	root6 := t.TempDir()
	writeRawRunShape(t, root6, "exec_sandbox:\n  min_mode: strict\nlifecycle: [unclosed bracket\n")
	if _, err := LoadMinMode(root6); err == nil {
		t.Fatalf("document syntax error: expected fail-closed error, got nil")
	}
}

// TestFindMinMode locks the walk-up locator: an exec-sandbox invocation from a
// SUBDIRECTORY still discovers the enclosing project's strict floor (closes the
// cwd-scoped bypass — a granted agent cannot escape the floor by cd-ing into a
// subdir). Mirrors FindForRoot's upward walk.
func TestFindMinMode(t *testing.T) {
	// project root carries the strict floor.
	project := t.TempDir()
	writeRawRunShape(t, project, "exec_sandbox:\n  min_mode: strict\n")

	// direct lookup from the project root => strict.
	pr, mm, err := FindMinMode(project)
	if err != nil || mm != "strict" {
		t.Fatalf("FindMinMode(project): root=%q mode=%q err=%v, want strict/nil", pr, mm, err)
	}

	// lookup from a nested subdir walks UP and finds the project floor.
	sub := filepath.Join(project, "internal", "cli")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatalf("mkdir subdir: %v", err)
	}
	pr2, mm2, err := FindMinMode(sub)
	if err != nil || mm2 != "strict" {
		t.Fatalf("FindMinMode(subdir) should walk up to the project floor: root=%q mode=%q err=%v, want strict/nil", pr2, mm2, err)
	}
	if pr2 != project {
		t.Fatalf("FindMinMode(subdir) root=%q, want project root %q", pr2, project)
	}

	// CHILD WITHOUT exec_sandbox MUST NOT MASK PARENT FLOOR: a child run-shape.yml
	// carrying only runtime/lifecycle config (no exec_sandbox block) must NOT stop
	// the walk. The search continues upward and discovers the enclosing parent's
	// strict floor — the containment-safe choice (a child without a floor
	// declaration must not silently disable an enclosing parent floor).
	childDir := filepath.Join(project, "nested", "child")
	if err := os.MkdirAll(filepath.Join(childDir, DirName), 0o755); err != nil {
		t.Fatalf("mkdir child vh-agent-harness: %v", err)
	}
	writeRawRunShape(t, childDir, "runtime:\n  backend: host-shell\n") // NO exec_sandbox
	prChild, mmChild, err := FindMinMode(childDir)
	if err != nil || mmChild != "strict" {
		t.Fatalf("FindMinMode(child without exec_sandbox) must walk past the child and find the parent strict floor: root=%q mode=%q err=%v, want strict/nil", prChild, mmChild, err)
	}
	if prChild != project {
		t.Fatalf("FindMinMode(child without exec_sandbox) root=%q, want parent project root %q (child must not mask parent floor)", prChild, project)
	}

	// WEAKENING CHILD FLOOR MUST NOT MASK PARENT STRICT (F1 anti-escape): an
	// agent granted exec-sandbox could plant a weakening child run-shape.yml
	// (e.g. under the RW ./tmp tree) with exec_sandbox.min_mode: off, then
	// invoke from that child. The MAX-over-entire-chain walk MUST find the
	// parent's strict floor and override the child's weakening off — a child
	// can never weaken an enclosing parent's containment floor.
	weakChild := filepath.Join(project, "tmp", "evil")
	if err := os.MkdirAll(filepath.Join(weakChild, DirName), 0o755); err != nil {
		t.Fatalf("mkdir weak child: %v", err)
	}
	writeRawRunShape(t, weakChild, "exec_sandbox:\n  min_mode: off\n") // weakening floor
	prWeak, mmWeak, err := FindMinMode(weakChild)
	if err != nil || mmWeak != "strict" {
		t.Fatalf("FindMinMode(weakening child with min_mode=off) must find the parent strict floor via MAX-over-chain: root=%q mode=%q err=%v, want strict/nil", prWeak, mmWeak, err)
	}
	if prWeak != project {
		t.Fatalf("FindMinMode(weakening child) root=%q, want parent project root %q (weakening child must not mask strict parent)", prWeak, project)
	}

	// no run-shape anywhere up the tree => ("", "", nil).
	empty := t.TempDir()
	pr3, mm3, err := FindMinMode(empty)
	if err != nil || mm3 != "" || pr3 != "" {
		t.Fatalf("FindMinMode(empty): root=%q mode=%q err=%v, want empty/nil", pr3, mm3, err)
	}

	// present-but-wrong-type floor => error propagates (fail-closed).
	bad := t.TempDir()
	writeRawRunShape(t, bad, "exec_sandbox:\n  min_mode: [strict]\n")
	if _, _, err := FindMinMode(bad); err == nil {
		t.Fatalf("FindMinMode(bad type): expected error to propagate, got nil")
	}

	// SYMLINK BYPASS CLOSED: an out-of-tree symlink targeting a nested project
	// dir must NOT escape the floor. Without EvalSymlinks, os.Getwd-style logical
	// paths would walk the symlink's parents (outside the repo) and find no floor
	// → ModeOff. FindMinMode canonicalizes to the physical path first, so the
	// walk ascends the REAL project tree and finds the strict floor.
	symProject := t.TempDir()
	writeRawRunShape(t, symProject, "exec_sandbox:\n  min_mode: strict\n")
	symSub := filepath.Join(symProject, "internal", "cli")
	if err := os.MkdirAll(symSub, 0o755); err != nil {
		t.Fatalf("mkdir symlink target: %v", err)
	}
	outside := t.TempDir()
	link := filepath.Join(outside, "into-project")
	if err := os.Symlink(symSub, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	prSym, mmSym, err := FindMinMode(link)
	if err != nil || mmSym != "strict" {
		t.Fatalf("FindMinMode(out-of-tree symlink into project): must resolve to the physical project strict floor; got root=%q mode=%q err=%v", prSym, mmSym, err)
	}
	if prSym != symProject {
		t.Fatalf("FindMinMode(out-of-tree symlink): resolved root=%q, want physical project root %q (symlink bypass not closed)", prSym, symProject)
	}

	// FAIL-CLOSED: a DIRECTORY at the floor path is malformed-present, NOT
	// absent — FindMinMode must return an error (not walk past it to ModeOff).
	// This is the D1 hardening: the walk guard distinguishes IsNotExist (absent
	// → walk up) from a directory/stat-error candidate (broken-present → fail).
	dirRepo := t.TempDir()
	dirPath := filepath.Join(dirRepo, DirName, FileName)
	if err := os.MkdirAll(dirPath, 0o755); err != nil {
		t.Fatalf("mkdir floor-as-dir: %v", err)
	}
	if _, _, err := FindMinMode(dirRepo); err == nil {
		t.Fatalf("FindMinMode(directory at floor path): expected fail-closed error, got nil")
	}
	// And LoadMinMode directly must also fail-closed on the same directory.
	if _, err := LoadMinMode(dirRepo); err == nil {
		t.Fatalf("LoadMinMode(directory at floor path): expected fail-closed error, got nil")
	}

	// FAIL-CLOSED: an unreadable floor path (stat error other than not-exist).
	// chmod the PARENT dir (.vh-agent-harness/) to 0000 so os.Stat itself is
	// denied (non-traversable), directly exercising FindMinMode's default
	// stat-error switch branch — the load-bearing D1 fail-closed arm. Skip under
	// root (root bypasses DAC permissions, so the test would not trigger).
	if os.Geteuid() != 0 {
		permRepo := t.TempDir()
		writeRawRunShape(t, permRepo, "exec_sandbox:\n  min_mode: strict\n")
		permParent := filepath.Join(permRepo, DirName)
		if err := os.Chmod(permParent, 0o000); err != nil {
			t.Fatalf("chmod parent: %v", err)
		}
		// Restore before TempDir's own cleanup removes the tree (LIFO: this runs
		// first, so the dir is traversable again for removal).
		t.Cleanup(func() { _ = os.Chmod(permParent, 0o755) })
		if _, _, err := FindMinMode(permRepo); err == nil {
			t.Fatalf("FindMinMode(unreadable parent dir): expected fail-closed error from the default stat-error branch, got nil")
		}
	}
}
