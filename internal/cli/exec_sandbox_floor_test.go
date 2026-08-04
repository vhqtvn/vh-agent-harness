package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

		// no floor (absent): the contained default (best-effort, the no-flag
		// value) and stricter modes are honored exactly (standalone behavior
		// PRESERVED by Fix 1). An explicit --sandbox=off is REFUSED — that case
		// is pinned in TestApplyFloorToRequest_AbsentFloorRefusesOff (it returns
		// an error, so it cannot live in this success-only table).
		{"no floor: best-effort+allow honored (standalone default)", noFloorRepo, execsandbox.ModeBestEffort, execsandbox.NetAllow, execsandbox.ModeBestEffort, execsandbox.NetAllow},
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
	// From the subdir, the floor must still resolve to strict (walk-up), and the
	// floor is PRESENT (an explicit min_mode was configured) — distinct from the
	// absent case (Fix 1).
	got, present, err := loadExecSandboxFloor(sub)
	if err != nil || got != execsandbox.ModeStrict {
		t.Fatalf("loadExecSandboxFloor(subdir): got %q err=%v, want strict/nil (walk-up must find project floor)", got, err)
	}
	if !present {
		t.Fatalf("loadExecSandboxFloor(subdir): present=false, want true (an explicit strict floor was configured; absent would be present=false)")
	}
}

// TestLoadExecSandboxFloor_FailClosed locks the B1 fail-closed contract at the
// CLI boundary: a present-but-broken floor (wrong-type min_mode, string typo,
// document syntax error) refuses to run uncontained rather than silently
// dropping to ModeOff. Absent floor => ModeOff with present=false (Fix 1: the
// absent case is carried as present=false so an explicit --sandbox=off can be
// refused upstream, distinct from an explicit min_mode: off which is
// present=true).
func TestLoadExecSandboxFloor_FailClosed(t *testing.T) {
	// absent floor => off, present=false (Fix 1 representation: absent is
	// distinct from explicit off).
	none := t.TempDir()
	got, present, err := loadExecSandboxFloor(none)
	if err != nil || got != execsandbox.ModeOff {
		t.Fatalf("absent floor: got %q err=%v, want off/nil", got, err)
	}
	if present {
		t.Fatalf("absent floor: present=true, want false (no exec_sandbox.min_mode block anywhere; absent must be distinguishable from explicit off)")
	}

	// valid strict => strict, present=true
	ok := t.TempDir()
	writeFloorRunShape(t, ok, "strict")
	got, present, err = loadExecSandboxFloor(ok)
	if err != nil || got != execsandbox.ModeStrict {
		t.Fatalf("strict floor: got %q err=%v, want strict/nil", got, err)
	}
	if !present {
		t.Fatalf("strict floor: present=false, want true")
	}

	// EXPLICIT off => ModeOff, present=true (Fix 1: deliberate opt-out is
	// PRESENT, distinct from absent). This is what still honors --sandbox=off.
	offRepo := t.TempDir()
	writeFloorRunShape(t, offRepo, "off")
	got, present, err = loadExecSandboxFloor(offRepo)
	if err != nil || got != execsandbox.ModeOff {
		t.Fatalf("explicit off floor: got %q err=%v, want off/nil", got, err)
	}
	if !present {
		t.Fatalf("explicit off floor: present=false, want true (explicit min_mode: off is a deliberate opt-out, distinct from absent)")
	}

	// string typo (VALUE) => fail-closed error
	typo := t.TempDir()
	writeFloorRunShape(t, typo, "strcit")
	if _, _, err := loadExecSandboxFloor(typo); err == nil {
		t.Fatalf("string typo min_mode value: expected fail-closed error, got nil")
	}

	// KEY typo (misspelled min_mode key) => fail-closed. yaml drops the unknown
	// key, leaving min_mode absent in a present block — the runtime refuses
	// rather than silently no-floor. This is the committer defense-in-depth
	// catch (key-typo hole).
	keyTypo := t.TempDir()
	writeFloorRunShapeRaw(t, keyTypo, "exec_sandbox:\n  min_mdoe: strict\n")
	if _, _, err := loadExecSandboxFloor(keyTypo); err == nil {
		t.Fatalf("misspelled min_mode key (min_mdoe): expected fail-closed error, got nil")
	}

	// present block, no min_mode (empty map) => fail-closed
	emptyBlock := t.TempDir()
	writeFloorRunShapeRaw(t, emptyBlock, "exec_sandbox: {}\n")
	if _, _, err := loadExecSandboxFloor(emptyBlock); err == nil {
		t.Fatalf("empty exec_sandbox block (no min_mode): expected fail-closed error, got nil")
	}

	// wrong-TYPE min_mode (sequence) => fail-closed (B1: not silently off)
	wt := t.TempDir()
	writeFloorRunShapeRaw(t, wt, "exec_sandbox:\n  min_mode: [strict]\n")
	if _, _, err := loadExecSandboxFloor(wt); err == nil {
		t.Fatalf("wrong-type min_mode (sequence): expected fail-closed error, got nil")
	}

	// document syntax error => fail-closed
	syn := t.TempDir()
	writeFloorRunShapeRaw(t, syn, "exec_sandbox:\n  min_mode: strict\nlifecycle: [unclosed\n")
	if _, _, err := loadExecSandboxFloor(syn); err == nil {
		t.Fatalf("document syntax error: expected fail-closed error, got nil")
	}

	// DIRECTORY at the floor path => fail-closed (D1: malformed-present must not
	// be treated as absent → ModeOff → uncontained Run).
	dirRepo := t.TempDir()
	dirAtFloor := filepath.Join(dirRepo, runshape.DirName, runshape.FileName)
	if err := os.MkdirAll(dirAtFloor, 0o755); err != nil {
		t.Fatalf("mkdir floor-as-dir: %v", err)
	}
	if _, _, err := loadExecSandboxFloor(dirRepo); err == nil {
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

	// Neither root has a floor → no clamp (standalone behavior). Use best-effort
	// (the no-flag default) — the legitimate standalone contained mode Fix 1
	// preserves. The absent + explicit --sandbox=off REFUSE is pinned below.
	mode3, net3, err := applyFloorToRequest(
		execsandbox.ModeBestEffort, execsandbox.NetAllow,
		outside, outside,
	)
	if err != nil {
		t.Fatalf("dual-root no-floor: unexpected error: %v", err)
	}
	if mode3 != execsandbox.ModeBestEffort || net3 != execsandbox.NetAllow {
		t.Fatalf("dual-root no-floor: mode=%q net=%q, want best-effort/allow (no floor = no clamp on the contained default)", mode3, net3)
	}

	// FIX 1 crux in the dual-root shape: when NEITHER root has a floor, an
	// explicit --sandbox=off downgrade is REFUSED (consistency with the
	// single-root refuse). Silence is not consent to disable containment.
	if _, _, err := applyFloorToRequest(
		execsandbox.ModeOff, execsandbox.NetAllow,
		outside, outside,
	); err == nil {
		t.Fatalf("dual-root no-floor + --sandbox=off: expected refuse error (Fix 1), got nil")
	}
}

// TestApplyFloorToRequest_FloorSafetyInvariant_FVBinding is the concrete
// proof↔code binding for the formal-verification S1 pilot
// (tmp/formal-verification-pilot/SandboxFloor.lean, checked green by Lean 4.32.2;
// see tmp/formal-verification-pilot/FIDELITY-BINDING.md).
//
// It materializes the Lean safety invariant
//
//	floor_le_effective : ∀ requested floor, rank floor ≤ rank (ApplyFloor requested floor)
//
// as an EXHAUSTIVE Go matrix over the full 3×3 (requested, floor) grid: for
// every pair, the effective mode's rank is ≥ the floor's rank — a caller can
// NEVER downgrade below the floor.
//
// RED-ON-DIVERGENCE: this test goes RED the moment ApplyFloor drops the max
// (flipped to min, or the floor simply ignored) — exactly the divergence the
// proof forbids. It was verified RED against a deliberately-broken ApplyFloor
// (max→min) before being left GREEN on the faithful implementation.
//
// ANTI-LAUNDERING: a proof of the model is not a proof of the code. The Lean
// theorem proves the modeled `max`; this test is the cheapest concrete recheck
// that the Go `ApplyFloor`/`applyFloorToRequest` upholds the never-below-floor
// HALF of that law (floor_le_effective: effective rank ≥ floor rank). It does
// NOT pin maximality (effective ≤ max(requested, floor)), so it would not catch
// an impl that always returns the floor/strict — only an impl that lets the
// caller downgrade below the floor, which is the load-bearing divergence.
// INFORM-only: never gates commits, releases, doctor, or update.
func TestApplyFloorToRequest_FloorSafetyInvariant_FVBinding(t *testing.T) {
	// rankOf mirrors execsandbox.modeRank (internal/execsandbox/floor.go) and the
	// Lean `rank` (SandboxFloor.lean): off < best-effort < strict.
	rankOf := func(m execsandbox.SandboxMode) int {
		switch m {
		case execsandbox.ModeStrict:
			return 2
		case execsandbox.ModeBestEffort:
			return 1
		case execsandbox.ModeOff:
			return 0
		}
		return 0
	}

	// One fixture per floor value, exactly as the Lean `floor` argument ranges
	// over {off, bestEffort, strict}. The Lean model's `floor` is an EXPLICIT
	// floor value — "off" here is a deliberate min_mode: off floor (present),
	// NOT the key-absent case. The absent case is a Go-runtime policy concern
	// (no floor configured → refuse explicit --sandbox=off, pinned separately
	// in TestApplyFloorToRequest_AbsentFloorRefusesOff); it is not part of the
	// Lean `floor` domain and is correctly excluded from this FV matrix.
	floorFixture := map[execsandbox.SandboxMode]string{
		execsandbox.ModeOff:        "off",
		execsandbox.ModeBestEffort: "best-effort",
		execsandbox.ModeStrict:     "strict",
	}
	roots := make(map[execsandbox.SandboxMode]string, len(floorFixture))
	for floor, val := range floorFixture {
		root := t.TempDir()
		if val != "" {
			writeFloorRunShape(t, root, val)
		}
		roots[floor] = root
	}

	// The full 3×3 matrix the Lean `floor_le_effective_all_pairs` evaluates.
	for _, floor := range []execsandbox.SandboxMode{execsandbox.ModeOff, execsandbox.ModeBestEffort, execsandbox.ModeStrict} {
		for _, requested := range []execsandbox.SandboxMode{execsandbox.ModeOff, execsandbox.ModeBestEffort, execsandbox.ModeStrict} {
			name := fmt.Sprintf("requested=%s/floor=%s", requested, floor)
			t.Run(name, func(t *testing.T) {
				effective, _, err := applyFloorToRequest(requested, execsandbox.NetDeny, roots[floor], roots[floor])
				if err != nil {
					t.Fatalf("applyFloorToRequest: unexpected error: %v", err)
				}
				// THE INVARIANT (Lean: rank floor ≤ rank effective).
				if rankOf(effective) < rankOf(floor) {
					t.Errorf("FV binding violated: requested=%s floor=%s -> effective=%s; "+
						"rank(effective)=%d < rank(floor)=%d (caller downgraded below the floor; "+
						"ApplyFloor must be MAX, not MIN)",
						requested, floor, effective, rankOf(effective), rankOf(floor))
				}
			})
		}
	}
}

// TestApplyFloorToRequest_AbsentFloorRefusesOff is the CRUX regression pin for
// FIX 1: when NO exec_sandbox.min_mode floor is configured (absent from the
// entire ancestor chain), an explicit caller downgrade to --sandbox=off is
// REFUSED. Before Fix 1, ParseMinMode collapsed absent and explicit-off to the
// same ModeOff, so applyFloorToRequest(Off, ...) silently honored the downgrade
// — meaning an exec-sandbox grant to a read-only agent became FULLY UNCONTAINED
// on an explicit --sandbox=off in any unfloored consumer (empirically the
// command exited 0 and wrote files outside ./tmp/). The refuse is what closes
// that hole: silence is not consent to disable containment. Because the command
// never reaches execsandbox.Run, the write outside ./tmp/ never happens — the
// unit-level pin is the refuse error from the clamp pipeline.
//
// This is the load-bearing path for the behavioral-closure crux declaration.
func TestApplyFloorToRequest_AbsentFloorRefusesOff(t *testing.T) {
	// no-floor repo (no run-shape anywhere in the chain)
	noFloorRepo := t.TempDir()

	// PIN 1 (Fix 1 crux): absent floor + explicit --sandbox=off → REFUSED.
	// Today (pre-Fix-1) this returned (ModeOff, NetAllow, nil) and the command
	// ran uncontained, creating files outside ./tmp/. After Fix 1 it errors.
	_, _, err := applyFloorToRequest(execsandbox.ModeOff, execsandbox.NetAllow, noFloorRepo, noFloorRepo)
	if err == nil {
		t.Fatalf("absent floor + --sandbox=off: expected refuse error (Fix 1 crux), got nil — the hole is still open (an explicit downgrade becomes fully uncontained in an unfloored repo)")
	}
	if !strings.Contains(err.Error(), "refusing --sandbox=off") {
		t.Fatalf("absent floor + --sandbox=off: error %q does not name the refuse (must mention 'refusing --sandbox=off' so the operator can act)", err.Error())
	}

	// PIN 3 (no regression to standalone): absent floor + NO flag (best-effort,
	// the no-flag default) → still CONTAINED (honored, no error, no clamp).
	// Fix 1 must NOT change standalone contained behavior.
	mode, net, err := applyFloorToRequest(execsandbox.ModeBestEffort, execsandbox.NetAllow, noFloorRepo, noFloorRepo)
	if err != nil {
		t.Fatalf("absent floor + best-effort (no-flag default): unexpected error %v (standalone contained behavior must be unchanged)", err)
	}
	if mode != execsandbox.ModeBestEffort || net != execsandbox.NetAllow {
		t.Fatalf("absent floor + best-effort: mode=%q net=%q, want best-effort/allow (contained default preserved)", mode, net)
	}

	// Absent floor + explicit strict → still contained (honored). Stricter
	// modes are never a downgrade and must remain honored.
	mode2, _, err := applyFloorToRequest(execsandbox.ModeStrict, execsandbox.NetDeny, noFloorRepo, noFloorRepo)
	if err != nil {
		t.Fatalf("absent floor + strict: unexpected error %v", err)
	}
	if mode2 != execsandbox.ModeStrict {
		t.Fatalf("absent floor + strict: mode=%q, want strict", mode2)
	}
}

// TestApplyFloorToRequest_ExplicitOffFloorHonored pins FIX 1's preserved
// opt-out: an EXPLICIT `min_mode: off` in run-shape.yml is a deliberate opt-out
// and STILL honors --sandbox=off. This is the counterpart to the absent refuse
// — the distinction Fix 1 draws is absent (refuse) vs explicit-off (honor),
// NOT off-refused-vs-off-honored. Without this pin a too-broad Fix 1 that
// refused all --sandbox=off would regress the deliberate opt-out.
func TestApplyFloorToRequest_ExplicitOffFloorHonored(t *testing.T) {
	// explicit min_mode: off floor (present, deliberate opt-out)
	offRepo := t.TempDir()
	writeFloorRunShape(t, offRepo, "off")

	// PIN 2: explicit min_mode: off + --sandbox=off → honored (opt-out preserved).
	mode, net, err := applyFloorToRequest(execsandbox.ModeOff, execsandbox.NetAllow, offRepo, offRepo)
	if err != nil {
		t.Fatalf("explicit min_mode: off + --sandbox=off: unexpected error %v (deliberate opt-out must still be honored)", err)
	}
	if mode != execsandbox.ModeOff || net != execsandbox.NetAllow {
		t.Fatalf("explicit off floor + --sandbox=off: mode=%q net=%q, want off/allow (deliberate opt-out preserved)", mode, net)
	}

	// Sanity: an explicit off floor is reported present by the loader (distinct
	// from absent), which is what makes the opt-out path reachable.
	_, present, err := loadExecSandboxFloor(offRepo)
	if err != nil || !present {
		t.Fatalf("explicit off floor: present=%v err=%v, want present=true/nil (distinct from absent)", present, err)
	}

	// And an absent floor is present=false (the distinction Fix 1 hinges on).
	absent := t.TempDir()
	_, presentAbsent, err := loadExecSandboxFloor(absent)
	if err != nil || presentAbsent {
		t.Fatalf("absent floor: present=%v err=%v, want present=false/nil", presentAbsent, err)
	}
}

// TestApplyFloorToRequest_InvalidFloorFailsClosed re-pins behavior 4 at the
// applyFloorToRequest boundary: a present-but-invalid min_mode (a value typo)
// must FAIL CLOSED (refuse to run uncontained), NOT silently collapse to
// ModeOff. This is the fail-closed contract that pre-dates Fix 1 and must not
// regress: Fix 1 refuses absent+off, but an invalid floor is a different
// (already-closed) failure that still errors.
func TestApplyFloorToRequest_InvalidFloorFailsClosed(t *testing.T) {
	// invalid min_mode VALUE (a typo) → fail-closed at the loader → propagates
	// through applyFloorToRequest as an error (never an uncontained run).
	typo := t.TempDir()
	writeFloorRunShape(t, typo, "strcit")
	if _, _, err := applyFloorToRequest(execsandbox.ModeBestEffort, execsandbox.NetDeny, typo, typo); err == nil {
		t.Fatalf("invalid min_mode value (strcit): expected fail-closed error from applyFloorToRequest, got nil (the operator asked for a floor we cannot honor; must refuse uncontained)")
	}
}
