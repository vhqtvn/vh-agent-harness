// workdir_test.go — TB-F1 round-2 (same-class hole): the run_shell
// workdir argument is model/client-controlled and steers WHERE the
// command executes. Policy under test (Config.WorkdirRoots):
//
//   - empty workdir allowed (engine CWD);
//   - relative workdir allowed only when lexically inside the engine
//     working directory (no leading "..");
//   - absolute workdir rejected by default, admitted only when it
//     resolves symlink-safe inside a configured WorkdirRoots entry.
package shell

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// runInCWDSubdir creates a real subdirectory under the CURRENT working
// directory (the confinement root for relative workdirs) and returns
// its relative path.
func runInCWDSubdir(t *testing.T) string {
	t.Helper()
	rel := filepath.Join("tmp-workdir-policy-test", t.Name())
	if err := os.MkdirAll(rel, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", rel, err)
	}
	t.Cleanup(func() { _ = os.RemoveAll("tmp-workdir-policy-test") })
	return rel
}

func TestWorkdirPolicyDefaults(t *testing.T) {
	cfg := Config{}
	cfg.normalize()

	// Empty workdir: allowed (engine CWD — pre-existing behavior).
	if err := validateWorkdir(&cfg, ""); err != nil {
		t.Fatalf("empty workdir rejected: %v", err)
	}

	// Relative inside CWD: allowed.
	rel := runInCWDSubdir(t)
	if err := validateWorkdir(&cfg, rel); err != nil {
		t.Fatalf("in-CWD relative workdir %q rejected: %v", rel, err)
	}

	// Relative escaping via leading "..": rejected.
	for _, wd := range []string{"..", "../" + rel, "sub/../../.." + "/etc"} {
		if err := validateWorkdir(&cfg, wd); err == nil {
			t.Fatalf("escaping relative workdir %q admitted", wd)
		} else if !strings.Contains(err.Error(), "confinement policy") {
			t.Fatalf("workdir %q error = %v, want confinement policy text", wd, err)
		}
	}

	// Absolute workdir: rejected by default (conservative posture),
	// including existing ones like the temp dir.
	abs := t.TempDir()
	if err := validateWorkdir(&cfg, abs); err == nil {
		t.Fatalf("absolute workdir %q admitted without any configured root", abs)
	} else if !strings.Contains(err.Error(), "WorkdirRoots") {
		t.Fatalf("absolute workdir error = %v, want it to name WorkdirRoots", err)
	}

	// Nonexistent relative workdir still fails the existence check.
	if err := validateWorkdir(&cfg, "no-such-dir-under-cwd"); err == nil {
		t.Fatal("nonexistent relative workdir admitted")
	}
}

func TestWorkdirPolicyRoots(t *testing.T) {
	rootParent := t.TempDir()
	root := filepath.Join(rootParent, "allowed-root")
	if err := os.MkdirAll(filepath.Join(root, "sub"), 0o755); err != nil {
		t.Fatalf("mkdir root: %v", err)
	}
	outside := filepath.Join(rootParent, "outside")
	if err := os.Mkdir(outside, 0o755); err != nil {
		t.Fatalf("mkdir outside: %v", err)
	}
	// A symlink inside the root pointing OUTSIDE.
	link := filepath.Join(root, "escape-link")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	cfg := Config{WorkdirRoots: []string{root}}
	cfg.normalize()

	// Absolute inside the configured root: allowed (incl. nested).
	for _, wd := range []string{root, filepath.Join(root, "sub")} {
		if err := validateWorkdir(&cfg, wd); err != nil {
			t.Fatalf("workdir %q under configured root rejected: %v", wd, err)
		}
	}

	// Absolute outside the root, the root's parent, and through a
	// symlinked dir inside the root: all rejected (symlink-safe).
	for _, wd := range []string{outside, rootParent, link, filepath.Join(link, "deeper")} {
		if err := validateWorkdir(&cfg, wd); err == nil {
			t.Fatalf("workdir %q admitted despite escaping the configured root", wd)
		}
	}

	// Relative escaping still rejected even with roots configured.
	if err := validateWorkdir(&cfg, "../evil"); err == nil {
		t.Fatal("escaping relative workdir admitted while roots configured")
	}
}

// TestWorkdirPolicyExecutes enforces the policy at the EXECUTE seam (the
// wire-facing tool body), not just the validator: an absolute workdir
// with the default config is an isError-class typed error before any
// process spawns.
func TestWorkdirPolicyExecutes(t *testing.T) {
	cfg := Config{}
	cfg.normalize()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	abs := t.TempDir()
	if _, err := execute(ctx, &cfg, []byte(`{"command":"pwd","workdir":"`+abs+`"}`)); err == nil {
		t.Fatal("execute with absolute workdir under default config succeeded: want rejection")
	} else if !strings.Contains(err.Error(), "confinement policy") {
		t.Fatalf("execute workdir error = %v, want confinement policy text", err)
	}

	rel := runInCWDSubdir(t)
	if _, err := execute(ctx, &cfg, []byte(`{"command":"pwd","workdir":"`+rel+`"}`)); err != nil {
		t.Fatalf("execute with in-CWD relative workdir failed: %v", err)
	}
}
