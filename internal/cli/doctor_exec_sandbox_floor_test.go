package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vhqtvn/vh-agent-harness/internal/permconfig"
	"github.com/vhqtvn/vh-agent-harness/internal/runshape"
)

// Tests for doctor check #22 exec-sandbox-floor (FIX-3 advisory). Coverage:
//   - WARN fires on grant-without-floor (the pinned behavior 5).
//   - PASS when a floor resolves (any value, incl. explicit off).
//   - SKIP when no agent carries the grant.
//   - SKIP when opencode.jsonc is absent/unparseable.
//   - ADVISORY ONLY: never tierFail across every reachable branch.

// writeOpencodeWithGrant writes a minimal opencode.jsonc whose agent block grants
// exec-sandbox to the named agents (the rendered form of
// permconfig.ReadOnlyExtraAllows: permission.bash[ExecSandboxCommand]="allow").
func writeOpencodeWithGrant(t *testing.T, dir string, grantedAgents []string) {
	t.Helper()
	var b strings.Builder
	b.WriteString("{\n  \"agent\": {\n")
	for i, a := range grantedAgents {
		if i > 0 {
			b.WriteString(",\n")
		}
		b.WriteString("    \"" + a + "\": {\n")
		b.WriteString("      \"permission\": {\n")
		b.WriteString("        \"bash\": {\n")
		b.WriteString("          \"" + permconfig.ExecSandboxCommand + "\": \"allow\"\n")
		b.WriteString("        }\n")
		b.WriteString("      }\n")
		b.WriteString("    }")
	}
	b.WriteString("\n  }\n}\n")
	if err := os.WriteFile(filepath.Join(dir, "opencode.jsonc"), []byte(b.String()), 0o644); err != nil {
		t.Fatalf("write opencode.jsonc: %v", err)
	}
}

// writeFloorAtRepo writes a run-shape.yml with the given exec_sandbox.min_mode
// under <repo>/.vh-agent-harness/ so FindMinMode resolves a floor for repo.
func writeFloorAtRepo(t *testing.T, repo, minMode string) {
	t.Helper()
	dir := filepath.Join(repo, runshape.DirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	body := "runtime: {backend: host-shell}\nexec_sandbox:\n  min_mode: " + minMode + "\n"
	if err := os.WriteFile(filepath.Join(dir, runshape.FileName), []byte(body), 0o644); err != nil {
		t.Fatalf("write run-shape: %v", err)
	}
}

// TestCheckExecSandboxGrantFloor_Unit is the FIX-3 crux (pinned behavior 5):
// grant carried + no floor => WARN, and the check is ADVISORY ONLY (never FAIL)
// across every reachable branch.
func TestCheckExecSandboxGrantFloor_Unit(t *testing.T) {
	// PIN 5 (Fix 3): grant carried + NO floor resolves => WARN.
	t.Run("grant_without_floor_WARN", func(t *testing.T) {
		dir := t.TempDir()
		writeOpencodeWithGrant(t, dir, []string{"researcher", "repo-explorer"})
		// no run-shape anywhere (absent floor)
		r := checkExecSandboxGrantFloor(dir)
		if r.tier != tierWarn {
			t.Fatalf("grant+no-floor: tier=%q, want WARN (Fix 3 advisory; got detail: %s)", r.tier, r.detail)
		}
		// ADVISORY ONLY contract: never FAIL.
		if r.tier == tierFail {
			t.Fatalf("grant+no-floor: tier=FAIL — the check MUST be advisory-only (never FAIL)")
		}
		// The WARN must name the durable fix so the operator can act.
		if !strings.Contains(r.detail, "min_mode") {
			t.Fatalf("grant+no-floor WARN detail should name the durable fix (min_mode); got: %s", r.detail)
		}
	})

	// PASS: grant carried AND a floor resolves (strict). The repo is durably
	// contained; no advisory needed.
	t.Run("grant_with_strict_floor_PASS", func(t *testing.T) {
		dir := t.TempDir()
		writeOpencodeWithGrant(t, dir, []string{"researcher"})
		writeFloorAtRepo(t, dir, "strict")
		r := checkExecSandboxGrantFloor(dir)
		if r.tier != tierPass {
			t.Fatalf("grant+strict-floor: tier=%q, want PASS (durable floor present; got: %s)", r.tier, r.detail)
		}
	})

	// PASS: grant carried AND an EXPLICIT off floor resolves (deliberate
	// opt-out). A conscious opt-out is a resolved floor posture, not the
	// absent case — the advisory must not fire on it.
	t.Run("grant_with_explicit_off_floor_PASS", func(t *testing.T) {
		dir := t.TempDir()
		writeOpencodeWithGrant(t, dir, []string{"researcher"})
		writeFloorAtRepo(t, dir, "off")
		r := checkExecSandboxGrantFloor(dir)
		if r.tier != tierPass {
			t.Fatalf("grant+explicit-off-floor: tier=%q, want PASS (deliberate opt-out is a resolved floor; got: %s)", r.tier, r.detail)
		}
	})

	// SKIP: no agent carries the grant (the advisory is moot).
	t.Run("no_grant_SKIP", func(t *testing.T) {
		dir := t.TempDir()
		// opencode.jsonc with an agent that has NO exec-sandbox allow.
		if err := os.WriteFile(filepath.Join(dir, "opencode.jsonc"),
			[]byte(`{"agent":{"planner":{"permission":{"bash":{"vh-agent-harness doctor":"allow"}}}}}`), 0o644); err != nil {
			t.Fatalf("write opencode.jsonc: %v", err)
		}
		r := checkExecSandboxGrantFloor(dir)
		if r.tier != tierSkip {
			t.Fatalf("no-grant: tier=%q, want SKIP (advisory moot; got: %s)", r.tier, r.detail)
		}
	})

	// SKIP: no opencode.jsonc (not installed / core-only).
	t.Run("no_config_SKIP", func(t *testing.T) {
		dir := t.TempDir()
		r := checkExecSandboxGrantFloor(dir)
		if r.tier != tierSkip {
			t.Fatalf("no-config: tier=%q, want SKIP (got: %s)", r.tier, r.detail)
		}
	})

	// ADVISORY-ONLY invariant: exercise every reachable branch and confirm NONE
	// produces tierFail (the check is the SAFETY LAYER INFORMing, never acting).
	t.Run("never_FAIL", func(t *testing.T) {
		scenarios := []struct {
			name  string
			setup func(t *testing.T) string
		}{
			{"grant+no-floor", func(t *testing.T) string {
				d := t.TempDir()
				writeOpencodeWithGrant(t, d, []string{"researcher"})
				return d
			}},
			{"grant+strict-floor", func(t *testing.T) string {
				d := t.TempDir()
				writeOpencodeWithGrant(t, d, []string{"researcher"})
				writeFloorAtRepo(t, d, "strict")
				return d
			}},
			{"grant+off-floor", func(t *testing.T) string {
				d := t.TempDir()
				writeOpencodeWithGrant(t, d, []string{"researcher"})
				writeFloorAtRepo(t, d, "off")
				return d
			}},
			{"no-grant", func(t *testing.T) string {
				d := t.TempDir()
				_ = os.WriteFile(filepath.Join(d, "opencode.jsonc"),
					[]byte(`{"agent":{"planner":{"permission":{"bash":{}}}}}`), 0o644)
				return d
			}},
			{"empty", func(t *testing.T) string { return t.TempDir() }},
		}
		for _, sc := range scenarios {
			t.Run(sc.name, func(t *testing.T) {
				r := checkExecSandboxGrantFloor(sc.setup(t))
				if r.tier == tierFail {
					t.Fatalf("branch %q produced tierFail — the check MUST be advisory-only (never FAIL); detail: %s", sc.name, r.detail)
				}
			})
		}
	})
}

// TestCheckExecSandboxGrantFloor_RunDoctor confirms the WARN surfaces in a real
// runDoctor report (the integration crux: the check is wired into runDoctor and
// its WARN does NOT make doctor UNHEALTHY — advisory only).
func TestCheckExecSandboxGrantFloor_RunDoctor(t *testing.T) {
	dir := t.TempDir()
	writeOpencodeWithGrant(t, dir, []string{"researcher"})
	// no run-shape -> grant carried, no floor.

	var out string
	runWithCwd(t, dir, func() {
		doctorTargetFlag = dir
		defer func() { doctorTargetFlag = "" }()
		cmd, buf := newOutCmd()
		_ = runDoctor(cmd, []string{})
		out = buf.String()
	})

	// The exec-sandbox-floor section must appear and WARN.
	if !strings.Contains(out, "exec-sandbox-floor:") {
		t.Fatalf("doctor output missing exec-sandbox-floor section:\n%s", out)
	}
	if !strings.Contains(out, "exec-sandbox-floor WARN") {
		t.Fatalf("grant+no-floor must surface exec-sandbox-floor WARN; got:\n%s", out)
	}
	// ADVISORY ONLY: the WARN must NOT add a FAIL. The synthetic tree lacks
	// lineage/etc. so other checks may report problems, but the summary's
	// warning count must include the exec-sandbox advisory (warns >= 1) and the
	// exec-sandbox-floor line itself must not be a FAIL.
	if strings.Contains(out, "exec-sandbox-floor FAIL") {
		t.Fatalf("exec-sandbox-floor line is FAIL — advisory contract violated:\n%s", out)
	}
}
