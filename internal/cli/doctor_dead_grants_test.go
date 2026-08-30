package cli

// ---------------------------------------------------------------------------
// Doctor dead-grants check tests (TrueAI defect 1a, PR A). Covers the
// OPERATOR-CONFIRMED O2 ACTION-KEYED severity mapping:
//   - dead allow AND dead ask grants  -> FAIL
//   - only redundant dead deny grants -> non-failing INFO
//   - zero findings                   -> quiet PASS
// plus the remediation-wording negative contract and the full-doctor
// integration (install -> transform-injected dead grant -> update -> doctor
// FAIL section).
// ---------------------------------------------------------------------------

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vhqtvn/vh-agent-harness/internal/permconfig"
)

// deadGrantsFixtureConfig carries a dead allow (build: "pytest *"), a dead ask
// (researcher: "make *"), a dead deny (default: "docker *"), and reachable
// canonical entries that MUST NOT be flagged (git verb, readonly, harness
// self-form, "*" wildcard).
const deadGrantsFixtureConfig = `{
    "agent": {
        "build": {
            "permission": {
                "bash": {
                    "*": "ask",
                    "git describe *": "allow",
                    "ls *": "allow",
                    "pytest *": "allow",
                    "vh-agent-harness *": "allow"
                }
            }
        },
        "researcher": {
            "permission": {
                "bash": {
                    "*": "deny",
                    "make *": "ask"
                }
            }
        }
    },
    "permission": {
        "bash": {
            "*": "deny",
            "docker *": "deny",
            "ls *": "allow"
        }
    }
}
`

// TestDeadGrants_FailOnDeadAllowAndAsk: a dead allow AND a dead ask grant must
// FAIL doctor (an ask is an interaction promise that can never fire), naming
// both agents + patterns + the remediation contract.
func TestDeadGrants_FailOnDeadAllowAndAsk(t *testing.T) {
	root := t.TempDir()
	writeOpencodeJSONC(t, root, deadGrantsFixtureConfig)
	res := checkDeadGrants(root)
	if res.tier != tierFail {
		t.Fatalf("tier = %q; want FAIL for dead allow + dead ask grants (detail=%q)", res.tier, res.detail)
	}
	for _, want := range []string{
		`agent "build"`, `"pytest *"`, `"allow"`,
		`agent "researcher"`, `"make *"`, `"ask"`,
		"hard-denied by shell-guard before the per-agent table is consulted",
		permconfig.DeadGrantRemediation,
	} {
		if !strings.Contains(res.detail, want) {
			t.Errorf("FAIL detail missing %q; got:\n%s", want, res.detail)
		}
	}
	// The redundant dead deny is counted but does not change the tier.
	if !strings.Contains(res.detail, "1 additional redundant dead deny grant") {
		t.Errorf("FAIL detail should note the redundant dead deny; got:\n%s", res.detail)
	}
	// Lint-scope clause (2026-08-31): grants denied only by project-owned
	// forbidden-pattern rules are outside the lint's model (the core engine
	// surface only) and must be stated as out of scope on the FAIL suffix.
	for _, want := range []string{"out of scope and not counted here", "project-owned forbidden-pattern rules"} {
		if !strings.Contains(res.detail, want) {
			t.Errorf("FAIL detail missing lint-scope clause %q; got:\n%s", want, res.detail)
		}
	}
}

// TestDeadGrants_InfoOnlyDeadDeny: when ONLY dead deny grants exist the check
// is non-failing INFO (the engine already denies; the entry is merely
// redundant).
func TestDeadGrants_InfoOnlyDeadDeny(t *testing.T) {
	root := t.TempDir()
	writeOpencodeJSONC(t, root, `{
    "agent": {
        "build": {
            "permission": {
                "bash": {
                    "*": "deny",
                    "docker *": "deny"
                }
            }
        }
    },
    "permission": {
        "bash": {
            "*": "deny"
        }
    }
}
`)
	res := checkDeadGrants(root)
	if res.tier != tierInfo {
		t.Fatalf("tier = %q; want INFO for dead-deny-only findings (detail=%q)", res.tier, res.detail)
	}
	if !strings.Contains(res.detail, `"docker *"`) {
		t.Errorf("INFO detail must name the redundant deny pattern; got:\n%s", res.detail)
	}
	// Lint-scope clause (2026-08-31): the INFO detail must state the lint's
	// scope so a downstream engine-grounded probe count larger than doctor's
	// (extra denies dead only under project-owned forbidden-pattern rules)
	// reads as out of scope, not unreconciled.
	for _, want := range []string{"out of scope and not counted here", "project-owned forbidden-pattern rules"} {
		if !strings.Contains(res.detail, want) {
			t.Errorf("INFO detail missing lint-scope clause %q; got:\n%s", want, res.detail)
		}
	}
}

// TestDeadGrants_HealthyQuietPass: zero findings is a quiet PASS — canonical
// entries only (git verbs, readonly group, gate group, harness self-forms,
// the "*" wildcard).
func TestDeadGrants_HealthyQuietPass(t *testing.T) {
	root := t.TempDir()
	writeOpencodeJSONC(t, root, `{
    "agent": {
        "build": {
            "permission": {
                "bash": {
                    "*": "ask",
                    "git diff *": "allow",
                    "git describe *": "allow",
                    "grep *": "allow",
                    "ls *": "allow",
                    ".opencode/scripts/commit-gate.sh status": "allow",
                    "vh-agent-harness *": "allow",
                    "vh-agent-harness exec-sandbox *": "allow"
                }
            }
        }
    },
    "permission": {
        "bash": {
            "*": "deny",
            "ls *": "allow"
        }
    }
}
`)
	res := checkDeadGrants(root)
	if res.tier != tierPass {
		t.Fatalf("tier = %q; want PASS on a clean table (detail=%q)", res.tier, res.detail)
	}
}

// TestDeadGrants_SkipWhenNoConfig: a missing opencode.jsonc is SKIP
// (managed-drift owns reporting the absence — mirrors checkOverlayPermissionState).
func TestDeadGrants_SkipWhenNoConfig(t *testing.T) {
	res := checkDeadGrants(t.TempDir())
	if res.tier != tierSkip {
		t.Fatalf("tier = %q; want SKIP with no opencode.jsonc (detail=%q)", res.tier, res.detail)
	}
}

// TestDeadGrants_FailOnUnparseableConfig: the emitter's output is always
// parseable, so a present-but-unparseable live config is a FAIL.
func TestDeadGrants_FailOnUnparseableConfig(t *testing.T) {
	root := t.TempDir()
	writeOpencodeJSONC(t, root, "{definitely not json")
	res := checkDeadGrants(root)
	if res.tier != tierFail {
		t.Fatalf("tier = %q; want FAIL on unparseable opencode.jsonc (detail=%q)", res.tier, res.detail)
	}
}

// TestDeadGrants_WordingContract: NEGATIVE assertions over every message
// surface the check emits (FAIL and INFO details). The operator-confirmed
// contract (2026-08-28): remediation text names ONLY remove-the-grant /
// downgrade-the-grant / route-through-vh-agent-harness-exec — NEVER
// engine-allowlist addition via overlay (no allow-side project seam exists) —
// and NEVER attributes a finding to an actively-executing agent (static
// configuration attribution only; the hook input exposes sessionID only).
func TestDeadGrants_WordingContract(t *testing.T) {
	failRoot := t.TempDir()
	writeOpencodeJSONC(t, failRoot, deadGrantsFixtureConfig)
	infoRoot := t.TempDir()
	writeOpencodeJSONC(t, infoRoot, `{
    "agent": {
        "build": {
            "permission": {
                "bash": {
                    "*": "deny",
                    "vh-agent-harness git *": "deny"
                }
            }
        }
    }
}
`)
	surfaces := []string{checkDeadGrants(failRoot).detail, checkDeadGrants(infoRoot).detail}
	forbidden := []string{
		// allowlist-addition suggestions (no allow-side project seam exists).
		"overlay", "allowlist", "allowed-commands", "forbidden-patterns", "permission-pack",
		// active-agent attribution / session-derived identity.
		"current agent", "active agent", "was denied by", "attempted", "session",
	}
	for i, surface := range surfaces {
		lower := strings.ToLower(surface)
		for _, bad := range forbidden {
			if strings.Contains(lower, bad) {
				t.Errorf("surface[%d] wording contract violation: contains forbidden token %q:\n%s", i, bad, surface)
			}
		}
	}
	// The FAIL surface must name the three sanctioned remediation paths.
	for _, want := range []string{"remove the grant", "downgrade the grant", "vh-agent-harness exec"} {
		if !strings.Contains(surfaces[0], want) {
			t.Errorf("FAIL detail must name remediation %q; got:\n%s", want, surfaces[0])
		}
	}
}

// TestDeadGrants_FullDoctorIntegration drives the real surface end-to-end:
// seam install into a scratch root, a config-transform that injects a dead
// allow (build: "pytest *") plus a dead ask (researcher: "make *"), a seam
// update that SUCCEEDS (warning only — emission is never blocked), and a full
// doctor run whose dead-grants section FAILs and reports UNHEALTHY. Requires
// Node (the transform runs through it).
func TestDeadGrants_FullDoctorIntegration(t *testing.T) {
	skipIfNoNode(t)
	root := t.TempDir()
	seamInstallInto(t, root)

	transform := `export default function() {
  return {
    permissionPatches: [
      { agent: "build", bash: [{ pattern: "pytest *", decision: "allow" }] },
      { agent: "researcher", bash: [{ pattern: "make *", decision: "ask" }] }
    ]
  };
}
`
	if err := os.MkdirAll(filepath.Join(root, ".vh-agent-harness"), 0o755); err != nil {
		t.Fatalf("mkdir .vh-agent-harness: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, ".vh-agent-harness", "config-transform.mjs"), []byte(transform), 0o644); err != nil {
		t.Fatalf("write transform: %v", err)
	}

	var updateErr error
	stderr := captureStderr(t, func() {
		_, updateErr = seamUpdateOut(t, root)
	})
	// CRUX 1: the update SUCCEEDS — the dead-grant lint is a WARNING on the
	// update path, never an emission hard error (install/update stay available
	// as repair paths).
	if updateErr != nil {
		t.Fatalf("update must succeed (warn, not block) with dead grants present; got %v", updateErr)
	}
	// CRUX 2: the warning fired on stderr naming agent + pattern + remediation.
	for _, want := range []string{
		"dead-lettered permission grant",
		`agent "build"`, `"pytest *"`,
		`agent "researcher"`, `"make *"`,
		permconfig.DeadGrantRemediation,
	} {
		if !strings.Contains(stderr, want) {
			t.Errorf("update stderr missing %q; got:\n%s", want, stderr)
		}
	}
	// The update-path warning surface obeys the same wording contract as the
	// doctor FAIL detail (review finding D2): no allowlist-addition suggestion
	// via overlay, no active-agent attribution.
	for _, bad := range []string{
		"overlay", "allowlist", "allowed-commands", "forbidden-patterns", "permission-pack",
		"current agent", "active agent", "was denied by", "attempted", "session",
	} {
		if strings.Contains(strings.ToLower(stderr), bad) {
			t.Errorf("update stderr wording violation: contains forbidden token %q;\n%s", bad, stderr)
		}
	}
	// The emitted config actually carries the grants (emission not blocked).
	liveCfg, err := os.ReadFile(filepath.Join(root, "opencode.jsonc"))
	if err != nil {
		t.Fatalf("read live opencode.jsonc: %v", err)
	}
	for _, want := range []string{`"pytest *": "allow"`, `"make *": "ask"`} {
		if !strings.Contains(string(liveCfg), want) {
			t.Errorf("live opencode.jsonc missing %s — the transform grant must be emitted", want)
		}
	}

	// CRUX 3: full doctor reports the dead-grants section as FAIL and the run
	// as UNHEALTHY.
	out := seamDoctorOut(t, root)
	if !strings.Contains(out, "dead-grants:") {
		t.Fatalf("doctor output missing the dead-grants section:\n%s", out)
	}
	sec := doctorSection(out, "dead-grants")
	if !strings.Contains(sec, "FAIL") {
		t.Errorf("dead-grants section must FAIL on dead allow+ask; got:\n%s", sec)
	}
	for _, want := range []string{`agent "build"`, `"pytest *"`, `agent "researcher"`, `"make *"`} {
		if !strings.Contains(sec, want) {
			t.Errorf("dead-grants section missing %q; got:\n%s", want, sec)
		}
	}
	if !strings.Contains(out, "result: UNHEALTHY") {
		t.Errorf("doctor must be UNHEALTHY with dead allow/ask grants; got:\n%s", out)
	}
}

// TestDeadGrants_FullDoctorHealthyOnCleanInstall: the dogfood-shaped negative
// control — a clean install (no dead grants) yields a PASS dead-grants section
// and no update-path warning.
func TestDeadGrants_FullDoctorHealthyOnCleanInstall(t *testing.T) {
	root := t.TempDir()
	var installStderr string
	installStderr = captureStderr(t, func() { seamInstallInto(t, root) })
	if strings.Contains(installStderr, "dead-lettered") {
		t.Errorf("clean install must not emit the dead-grant warning; got:\n%s", installStderr)
	}
	out := seamDoctorOut(t, root)
	sec := doctorSection(out, "dead-grants")
	if !strings.Contains(sec, "PASS") {
		t.Errorf("clean install's dead-grants section must PASS; got:\n%s", sec)
	}
	if strings.Contains(sec, "FAIL") || strings.Contains(sec, "WARN") {
		t.Errorf("clean install's dead-grants section must be quiet; got:\n%s", sec)
	}
}

// doctorSection extracts one doctor check's section body from a full runDoctor
// output (the two-space-indented block after "  <name>:").
func doctorSection(out, name string) string {
	lines := strings.Split(out, "\n")
	var body []string
	in := false
	for _, ln := range lines {
		if ln == "  "+name+":" {
			in = true
			continue
		}
		if in {
			if strings.HasPrefix(ln, "  ") && !strings.HasPrefix(ln, "    ") {
				break // next section
			}
			body = append(body, ln)
		}
	}
	return strings.Join(body, "\n")
}
