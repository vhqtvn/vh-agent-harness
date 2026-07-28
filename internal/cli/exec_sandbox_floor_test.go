package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/vhqtvn/vh-agent-harness/internal/execsandbox"
	"github.com/vhqtvn/vh-agent-harness/internal/runshape"
)

// writeFloorRunShape writes a run-shape.yml under root/.vh-agent-harness/ with the
// given exec_sandbox.min_mode value (empty => no exec_sandbox block).
func writeFloorRunShape(t *testing.T, root, minMode string) {
	t.Helper()
	dir := filepath.Join(root, runshape.DirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	var body string
	if minMode == "" {
		body = "runtime: {backend: host-shell}\n"
	} else {
		body = "runtime: {backend: host-shell}\nexec_sandbox:\n  min_mode: " + minMode + "\n"
	}
	if err := os.WriteFile(filepath.Join(dir, runshape.FileName), []byte(body), 0o644); err != nil {
		t.Fatalf("write run-shape: %v", err)
	}
}

// writeFloorRunShapeRaw writes arbitrary raw YAML as the run-shape (for the
// wrong-type / syntax-error fail-closed cases).
func writeFloorRunShapeRaw(t *testing.T, root, raw string) {
	t.Helper()
	dir := filepath.Join(root, runshape.DirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, runshape.FileName), []byte(raw), 0o644); err != nil {
		t.Fatalf("write run-shape: %v", err)
	}
}

// TestApplyFloorToRequest is the CRUX end-to-end-of-clamp-pipeline test (D1): it
// exercises the full path from flag-resolved (mode, net) values through floor
// resolution + clamp to the effective (mode, net) that would reach
// execsandbox.Run. This is the binary-side containment contract minus the kernel
// Run path (the kernel enforcement is the Slice 4 dogfood probes).
func TestApplyFloorToRequest(t *testing.T) {
	// strict floor repo
	strictRepo := t.TempDir()
	writeFloorRunShape(t, strictRepo, "strict")
	// best-effort floor repo
	beRepo := t.TempDir()
	writeFloorRunShape(t, beRepo, "best-effort")
	// no-floor repo (absent)
	noFloorRepo := t.TempDir()

	cases := []struct {
		name     string
		root     string
		reqMode  execsandbox.SandboxMode
		reqNet   execsandbox.NetPolicy
		wantMode execsandbox.SandboxMode
		wantNet  execsandbox.NetPolicy
	}{
		// P5 bypass + B2 net clamp, together: off+allow under strict → strict+deny.
		{"P5+b2: off+allow under strict -> strict+deny", strictRepo, execsandbox.ModeOff, execsandbox.NetAllow, execsandbox.ModeStrict, execsandbox.NetDeny},
		// strict floor raises mode but net already deny.
		{"strict floor: off+deny -> strict+deny", strictRepo, execsandbox.ModeOff, execsandbox.NetDeny, execsandbox.ModeStrict, execsandbox.NetDeny},
		// strict floor: best-effort+allow -> strict+deny (both clamped).
		{"strict floor: best-effort+allow -> strict+deny", strictRepo, execsandbox.ModeBestEffort, execsandbox.NetAllow, execsandbox.ModeStrict, execsandbox.NetDeny},
		// strict floor: already strict+deny, unchanged.
		{"strict floor: strict+deny unchanged", strictRepo, execsandbox.ModeStrict, execsandbox.NetDeny, execsandbox.ModeStrict, execsandbox.NetDeny},
		// strict floor: strict+ask -> strict+deny (ask upgraded to deny under strict).
		{"strict floor: strict+ask -> strict+deny", strictRepo, execsandbox.ModeStrict, execsandbox.NetAsk, execsandbox.ModeStrict, execsandbox.NetDeny},

		// best-effort floor: mode clamped up to best-effort, NET NOT clamped
		// (only a strict floor forces Level-B network denial).
		{"best-effort floor: off+allow -> best-effort+allow (net untouched)", beRepo, execsandbox.ModeOff, execsandbox.NetAllow, execsandbox.ModeBestEffort, execsandbox.NetAllow},
		{"best-effort floor: off+deny -> best-effort+deny", beRepo, execsandbox.ModeOff, execsandbox.NetDeny, execsandbox.ModeBestEffort, execsandbox.NetDeny},
		{"best-effort floor: strict+allow unchanged (caller stricter on mode)", beRepo, execsandbox.ModeStrict, execsandbox.NetAllow, execsandbox.ModeStrict, execsandbox.NetAllow},

		// no floor (absent): requested mode+net honored exactly (standalone).
		{"no floor: off+allow honored", noFloorRepo, execsandbox.ModeOff, execsandbox.NetAllow, execsandbox.ModeOff, execsandbox.NetAllow},
		{"no floor: strict+deny honored", noFloorRepo, execsandbox.ModeStrict, execsandbox.NetDeny, execsandbox.ModeStrict, execsandbox.NetDeny},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotMode, gotNet, err := applyFloorToRequest(tc.reqMode, tc.reqNet, tc.root, tc.root)
			if err != nil {
				t.Fatalf("applyFloorToRequest: unexpected error: %v", err)
			}
			if gotMode != tc.wantMode {
				t.Errorf("mode = %q, want %q", gotMode, tc.wantMode)
			}
			if gotNet != tc.wantNet {
				t.Errorf("net = %q, want %q", gotNet, tc.wantNet)
			}
		})
	}
}

// TestApplyFloorToRequest_P5BypassDenied is the named crux: under a strict
// floor, --sandbox=off (the P5 bypass that used to write outside tmp) is
// upgraded to strict, so the caller cannot run below the floor.
func TestApplyFloorToRequest_P5BypassDenied(t *testing.T) {
	root := t.TempDir()
	writeFloorRunShape(t, root, "strict")
	mode, net, err := applyFloorToRequest(execsandbox.ModeOff, execsandbox.NetDeny, root, root)
	if err != nil {
		t.Fatalf("applyFloorToRequest: %v", err)
	}
	if mode != execsandbox.ModeStrict {
		t.Fatalf("P5 bypass under strict floor: effective mode = %q, want strict (off must be upgraded)", mode)
	}
	if net != execsandbox.NetDeny {
		t.Fatalf("net = %q, want deny", net)
	}
}

// TestLoadExecSandboxFloor_WalkUp closes the B3 cwd-scoped bypass: the floor is
// discovered by walking UP from floorRoot, so an invocation from a subdirectory
// (or with --cwd under a subdir) still finds the enclosing project's strict
// floor. This is what makes the exec-sandbox grant safe regardless of the
// agent's working directory.
func TestLoadExecSandboxFloor_WalkUp(t *testing.T) {
	project := t.TempDir()
	writeFloorRunShape(t, project, "strict")
	sub := filepath.Join(project, "internal", "cli")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatalf("mkdir subdir: %v", err)
	}
	// From the subdir, the floor must still resolve to strict (walk-up).
	got, err := loadExecSandboxFloor(sub)
	if err != nil || got != execsandbox.ModeStrict {
		t.Fatalf("loadExecSandboxFloor(subdir): got %q err=%v, want strict/nil (walk-up must find project floor)", got, err)
	}
}

// TestLoadExecSandboxFloor_FailClosed locks the B1 fail-closed contract at the
// CLI boundary: a present-but-broken floor (wrong-type min_mode, string typo,
// document syntax error) refuses to run uncontained rather than silently
// dropping to ModeOff. Absent floor => ModeOff (no floor, run fine).
func TestLoadExecSandboxFloor_FailClosed(t *testing.T) {
	// absent floor => off (run fine)
	none := t.TempDir()
	got, err := loadExecSandboxFloor(none)
	if err != nil || got != execsandbox.ModeOff {
		t.Fatalf("absent floor: got %q err=%v, want off/nil", got, err)
	}

	// valid strict => strict
	ok := t.TempDir()
	writeFloorRunShape(t, ok, "strict")
	got, err = loadExecSandboxFloor(ok)
	if err != nil || got != execsandbox.ModeStrict {
		t.Fatalf("strict floor: got %q err=%v, want strict/nil", got, err)
	}

	// string typo (VALUE) => fail-closed error
	typo := t.TempDir()
	writeFloorRunShape(t, typo, "strcit")
	if _, err := loadExecSandboxFloor(typo); err == nil {
		t.Fatalf("string typo min_mode value: expected fail-closed error, got nil")
	}

	// KEY typo (misspelled min_mode key) => fail-closed. yaml drops the unknown
	// key, leaving min_mode absent in a present block — the runtime refuses
	// rather than silently no-floor. This is the committer defense-in-depth
	// catch (key-typo hole).
	keyTypo := t.TempDir()
	writeFloorRunShapeRaw(t, keyTypo, "exec_sandbox:\n  min_mdoe: strict\n")
	if _, err := loadExecSandboxFloor(keyTypo); err == nil {
		t.Fatalf("misspelled min_mode key (min_mdoe): expected fail-closed error, got nil")
	}

	// present block, no min_mode (empty map) => fail-closed
	emptyBlock := t.TempDir()
	writeFloorRunShapeRaw(t, emptyBlock, "exec_sandbox: {}\n")
	if _, err := loadExecSandboxFloor(emptyBlock); err == nil {
		t.Fatalf("empty exec_sandbox block (no min_mode): expected fail-closed error, got nil")
	}

	// wrong-TYPE min_mode (sequence) => fail-closed (B1: not silently off)
	wt := t.TempDir()
	writeFloorRunShapeRaw(t, wt, "exec_sandbox:\n  min_mode: [strict]\n")
	if _, err := loadExecSandboxFloor(wt); err == nil {
		t.Fatalf("wrong-type min_mode (sequence): expected fail-closed error, got nil")
	}

	// document syntax error => fail-closed
	syn := t.TempDir()
	writeFloorRunShapeRaw(t, syn, "exec_sandbox:\n  min_mode: strict\nlifecycle: [unclosed\n")
	if _, err := loadExecSandboxFloor(syn); err == nil {
		t.Fatalf("document syntax error: expected fail-closed error, got nil")
	}

	// DIRECTORY at the floor path => fail-closed (D1: malformed-present must not
	// be treated as absent → ModeOff → uncontained Run).
	dirRepo := t.TempDir()
	dirAtFloor := filepath.Join(dirRepo, runshape.DirName, runshape.FileName)
	if err := os.MkdirAll(dirAtFloor, 0o755); err != nil {
		t.Fatalf("mkdir floor-as-dir: %v", err)
	}
	if _, err := loadExecSandboxFloor(dirRepo); err == nil {
		t.Fatalf("directory at floor path: expected fail-closed error, got nil")
	}
}

// TestExecSandboxFlagResolution_DuplicateAndClamped proves that NO arrangement
// of --sandbox flags can downgrade below the floor: cobra/pflag resolves
// duplicate --sandbox (last-wins) to ONE value, and with SetInterspersed(false)
// a trailing --sandbox after the command is an ARGUMENT not a flag — so only
// leading flags set the mode, and ApplyFloor clamps whatever they resolve to.
func TestExecSandboxFlagResolution_DuplicateAndClamped(t *testing.T) {
	root := t.TempDir()
	writeFloorRunShape(t, root, "strict")

	resolveFlags := func(args []string) string {
		var m string
		c := &cobra.Command{
			Use: "exec-sandbox <command> [args...]",
			Run: func(cmd *cobra.Command, a []string) {},
		}
		c.Flags().SetInterspersed(false)
		c.Flags().StringVar(&m, "sandbox", "best-effort", "sandbox mode")
		c.SetArgs(args)
		if err := c.Execute(); err != nil {
			t.Fatalf("parse flags %v: %v", args, err)
		}
		return m
	}

	cases := []struct {
		name string
		args []string
		want string // resolved raw value before clamp
	}{
		{"single off before cmd", []string{"--sandbox=off", "--", "ls"}, "off"},
		{"duplicate off (last-wins)", []string{"--sandbox=off", "--sandbox=off", "--", "ls"}, "off"},
		{"off after strict (last-wins -> off)", []string{"--sandbox=strict", "--sandbox=off", "--", "ls"}, "off"},
		{"trailing strict is an arg (interspersed off)", []string{"--sandbox=off", "--", "ls", "--sandbox=strict"}, "off"},
		{"explicit strict", []string{"--sandbox=strict", "--", "ls"}, "strict"},
		{"default best-effort", []string{"--", "ls"}, "best-effort"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resolved := resolveFlags(tc.args)
			if resolved != tc.want {
				t.Fatalf("resolved --sandbox = %q, want %q", resolved, tc.want)
			}
			// Feed the resolved value through the clamp pipeline (mirrors
			// runExecSandbox: parseSandboxMode then applyFloorToRequest).
			req, perr := parseSandboxMode(resolved)
			if perr != nil {
				t.Fatalf("parseSandboxMode(%q): %v", resolved, perr)
			}
			eff, _, err := applyFloorToRequest(req, execsandbox.NetDeny, root, root)
			if err != nil {
				t.Fatalf("applyFloorToRequest: %v", err)
			}
			// CRUX invariant: under a strict floor, the effective mode is ALWAYS
			// strict, regardless of how many --sandbox flags the caller stacked
			// or where they put them.
			if eff != execsandbox.ModeStrict {
				t.Fatalf("effective mode under strict floor = %q (resolved=%q), want strict", eff, resolved)
			}
		})
	}
}

// TestExecSandboxFloor_InvalidModeRejected: a caller passing a bogus --sandbox
// value is rejected by parseSandboxMode BEFORE the floor logic runs (the floor
// never sees an unknown requested mode).
func TestExecSandboxFloor_InvalidModeRejected(t *testing.T) {
	if _, err := parseSandboxMode("nuclear"); err == nil {
		t.Fatalf("parseSandboxMode(nuclear): expected error, got nil")
	}
	if _, err := parseSandboxMode(""); err == nil {
		t.Fatalf("parseSandboxMode(empty): expected error, got nil")
	}
	for _, ok := range []string{"off", "best-effort", "strict"} {
		if _, err := parseSandboxMode(ok); err != nil {
			t.Fatalf("parseSandboxMode(%q): unexpected error %v", ok, err)
		}
	}
}

// TestApplyFloorToRequest_DualRootBypass closes the --cwd out-of-project bypass:
// a caller at /tmp (no floor) using --cwd to target a strict-floored project
// must STILL discover the project's floor. The floor is resolved from BOTH
// realCWD and repoRoot, taking the MAX — so a strict floor from EITHER root
// applies. Without dual-root resolution, the caller's cwd (/tmp) has no floor,
// and the --cwd target's strict floor is never consulted.
func TestApplyFloorToRequest_DualRootBypass(t *testing.T) {
	// strict-floored project
	project := t.TempDir()
	writeFloorRunShape(t, project, "strict")
	// caller cwd with NO floor (simulates /tmp)
	outside := t.TempDir()

	// CRUX: caller from outside, --cwd into the strict project → floor discovered
	// from repoRoot (project) even though realCWD (outside) has no floor.
	mode, net, err := applyFloorToRequest(
		execsandbox.ModeOff, execsandbox.NetAllow, // P5 bypass attempt
		outside, project, // realCWD=outside (no floor), repoRoot=project (strict)
	)
	if err != nil {
		t.Fatalf("dual-root: unexpected error: %v", err)
	}
	if mode != execsandbox.ModeStrict {
		t.Fatalf("dual-root bypass: effective mode = %q, want strict (repoRoot floor must apply even when realCWD has no floor)", mode)
	}
	if net != execsandbox.NetDeny {
		t.Fatalf("dual-root bypass: net = %q, want deny", net)
	}

	// Reverse: caller inside the strict project, --cwd outside → floor from
	// realCWD (project) still applies.
	mode2, net2, err := applyFloorToRequest(
		execsandbox.ModeOff, execsandbox.NetAllow,
		project, outside, // realCWD=project (strict), repoRoot=outside (no floor)
	)
	if err != nil {
		t.Fatalf("dual-root reverse: unexpected error: %v", err)
	}
	if mode2 != execsandbox.ModeStrict {
		t.Fatalf("dual-root reverse: mode = %q, want strict (realCWD floor must apply)", mode2)
	}
	if net2 != execsandbox.NetDeny {
		t.Fatalf("dual-root reverse: net = %q, want deny", net2)
	}

	// Neither root has a floor → no clamp (standalone behavior).
	mode3, net3, err := applyFloorToRequest(
		execsandbox.ModeOff, execsandbox.NetAllow,
		outside, outside,
	)
	if err != nil {
		t.Fatalf("dual-root no-floor: unexpected error: %v", err)
	}
	if mode3 != execsandbox.ModeOff || net3 != execsandbox.NetAllow {
		t.Fatalf("dual-root no-floor: mode=%q net=%q, want off/allow (no floor = no clamp)", mode3, net3)
	}
}
