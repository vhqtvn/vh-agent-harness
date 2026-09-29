package permconfig

// ---------------------------------------------------------------------------
// Inter-layer dead-grant lint tests (TrueAI defect 1a). See dead_grant_lint.go
// for the detection contract: a "dead grant" is a permission.bash entry whose
// configured action can never take effect because shell-guard hard-denies
// every command the pattern matches BEFORE the per-agent table is consulted.
//
// Test shapes (mirroring the dead_rule_lint_test.go discipline — a real-data
// clean pass cannot prove the detector fires, so positive-fire fixtures are
// mandatory):
//
//   - TestDeadGrantLint_RealTablesClean: the dogfood core tables (and, via
//     LintDeadGrants' resolveRules path, overlay packs) produce ZERO findings
//     — the no-false-positive contract on this repo's own tables.
//   - TestDeadGrantLint_Fixtures: synthesized positive-fire and negative
//     (must-not-flag) fixtures for every modeling caveat.
//   - TestDeadGrantLint_InConfig: the emitted-config view (agent attribution,
//     deterministic ordering, SourceLine resolution, transform-contributed
//     ExtraBash visibility).
//   - TestDeadGrantLint_WordingContract: negative assertions enforcing the
//     operator-confirmed remediation-text contract.
//   - TestDeadGrantLint_EngineParityByClass: per-class parity fixtures for
//     every evaluate() branch in shell-guard-core.js (the doc table in
//     dead_grant_lint.go), pinning the lint's verdict for the classes the
//     fixtures above miss — including the intentional under-approximations
//     (caveats d/e/f), which MUST stay not-flagged.
// ---------------------------------------------------------------------------

import (
	"strings"
	"testing"
)

// TestDeadGrantLint_RealTablesClean asserts the dogfood inventory is clean:
// NO real CoreLocationRules agent emits a dead grant, and the resolveRules
// path (core + packs) over zero packs agrees. This is the no-false-positive
// contract the card's success criteria pin ("zero false positives on this
// repo's own tables").
func TestDeadGrantLint_RealTablesClean(t *testing.T) {
	for agent := range CoreLocationRules {
		agent := agent
		t.Run(agent, func(t *testing.T) {
			for _, f := range LintDeadGrantsForAgent(CoreLocationRules, agent) {
				t.Errorf("%s (denyClass=%s)", f, f.DenyClass)
			}
		})
	}
	if findings := LintDeadGrants(nil); len(findings) != 0 {
		t.Fatalf("LintDeadGrants over core+zero packs must be clean; got %d finding(s): %v", len(findings), findings)
	}
}

// TestDeadGrantLint_Fixtures drives the detector over synthesized grant
// patterns via the rules-map seam (ExtraBash is the realistic authoring
// channel for non-canonical grants). Positive rows MUST fire; negative rows
// MUST NOT (each negative row names the modeling caveat it pins).
func TestDeadGrantLint_Fixtures(t *testing.T) {
	cases := []struct {
		pattern   string
		wantDead  bool
		wantClass string
		why       string
	}{
		// --- positive fire: dead grants -----------------------------------
		{pattern: "pytest *", wantDead: true, wantClass: DenyClassNonAllowlisted,
			why: "non-readonly command family outside the engine allowlist (the TrueAI defect class)"},
		{pattern: "make *", wantDead: true, wantClass: DenyClassNonAllowlisted,
			why: "dead ask fixture: build verb outside the read-only surface"},
		{pattern: "npm test *", wantDead: true, wantClass: DenyClassNonAllowlisted,
			why: "npm is not in any command group"},
		{pattern: "python3 tmp/x.py", wantDead: true, wantClass: DenyClassNonAllowlisted,
			why: "exact-form python invocation is engine-denied"},
		{pattern: "docker *", wantDead: true, wantClass: DenyClassNonAllowlisted,
			why: "dead deny fixture: redundant because the engine already denies"},
		{pattern: "terraform apply *", wantDead: true, wantClass: DenyClassNonAllowlisted,
			why: "infra lifecycle verb outside the read-only surface"},
		{pattern: "vh-agent-harness git *", wantDead: true, wantClass: DenyClassHarnessGit,
			why: "caveat (a) exception: harness git forms are hard-denied"},
		{pattern: "vh-agent-harness git status *", wantDead: true, wantClass: DenyClassHarnessGit,
			why: "longer harness-git wildcard form is also denied"},
		{pattern: "vh-agent-harness git push", wantDead: true, wantClass: DenyClassHarnessGit,
			why: "exact harness-git form with a trailing verb token is denied"},
		{pattern: "vh-agent-harness exec .opencode/scripts/commit-gate.sh *", wantDead: true, wantClass: DenyClassGateWrapper,
			why: "caveat (a) exception: gate-wrapper forms through exec are hard-denied"},
		{pattern: "vh-agent-harness exec bash -c '.opencode/scripts/commit-gate.sh acquire *'", wantDead: false,
			why: "quoted-payload pattern carries an interior '*' token: verdict-unknown (never flagged) — the safe-direction carve-out for glob semantics the token model does not represent; the unquoted gate-wrapper class above is still flagged"},

		// --- negative: must NOT flag (reachability + out-of-scope covenants) --
		{pattern: "git describe *", wantDead: false,
			why: "caveat (b): unknown git verb falls to engine ask -> table-rescuable"},
		{pattern: "git add *", wantDead: false,
			why: "caveat (c): git mutation verbs are forbidden-pattern denies, out of scope"},
		{pattern: "git diff *", wantDead: false,
			why: "git_readonly verb matches the engine allowlist"},
		{pattern: "vh-agent-harness update *", wantDead: false,
			why: "caveat (a): ordinary harness self-forms hit the auto-allow branch"},
		{pattern: "vh-agent-harness exec-sandbox *", wantDead: false,
			why: "harness self-form (exec-sandbox grant channel)"},
		{pattern: "vh-agent-harness defer-triggers", wantDead: false,
			why: "exact harness self-form is allowed"},
		{pattern: "vh-agent-harness git", wantDead: false,
			why: "bare exact form reaches the allowlist branch (deny needs a trailing token)"},
		{pattern: "vh-agent-harness exec *", wantDead: false,
			why: "exec family has engine-allowed members (non-gate payloads)"},
		{pattern: "vh-agent-harness exec bash -c *", wantDead: false,
			why: "exec + arbitrary payload without a commit-gate.sh token is allowed"},
		{pattern: "vh-agent-harness exec bash -n *commit-gate.sh", wantDead: false,
			why: "static-inspection grammar (bash -n) is carved out of the gate-wrapper deny"},
		{pattern: "vh-agent-harness exec cmp *", wantDead: false,
			why: "static-inspection grammar (cmp) is carved out"},
		{pattern: "vh-agent-harness accept-platform *", wantDead: false,
			why: "native path-operand verb never executes its operands"},
		{pattern: "ls *", wantDead: false,
			why: "readonly group member"},
		{pattern: "grep -F *", wantDead: false,
			why: "extends the readonly 'grep *' allowlist pattern"},
		{pattern: "sed *", wantDead: false,
			why: "wildcard grant intersects 'sed -n *' (engine strips nothing here; token-prefix compatible)"},
		{pattern: "jq", wantDead: false,
			why: "bare readonly binary matches 'jq *' with zero extra tokens"},
		{pattern: ".opencode/scripts/commit-gate.sh acquire *", wantDead: false,
			why: "gate group member (engine allowlists the direct invocation)"},
		{pattern: ".opencode/scripts/commit-gate.sh *", wantDead: false,
			why: "wildcard over the gate verbs intersects the enumerated gate patterns"},
		{pattern: "FOO=1 ls *", wantDead: false,
			why: "engine strips leading env-var assignments before the allowlist check"},
		{pattern: "*git *", wantDead: false,
			why: "non-terminal wildcard uses glob semantics the token model does not represent (verdict-unknown, never flagged)"},
		{pattern: "cat *=x *", wantDead: false,
			why: "interior wildcard token: verdict-unknown, never flagged"},
	}

	const fixtureAgent = "fixture-agent"
	rule := LocationRule{Wildcard: Deny, Readonly: Allow, GitReadonly: Allow, HasGate: false, HarnessPolicy: HarnessPolicyDeny}
	for _, c := range cases {
		c := c
		t.Run(c.pattern, func(t *testing.T) {
			r := rule
			r.ExtraBash = []BashEntry{{Pattern: c.pattern, Decision: Allow}}
			findings := LintDeadGrantsForAgent(map[string]LocationRule{fixtureAgent: r}, fixtureAgent)
			if c.wantDead {
				if len(findings) == 0 {
					t.Fatalf("pattern %q must be flagged dead (%s); got no findings", c.pattern, c.why)
				}
				f := findings[0]
				if f.Pattern != c.pattern || f.Agent != fixtureAgent || f.EntryValue != string(Allow) {
					t.Errorf("finding fields = {agent %q, pattern %q, value %q}; want {%q, %q, %q}",
						f.Agent, f.Pattern, f.EntryValue, fixtureAgent, c.pattern, string(Allow))
				}
				if f.DenyClass != c.wantClass {
					t.Errorf("pattern %q deny class = %q; want %q", c.pattern, f.DenyClass, c.wantClass)
				}
				return
			}
			for _, f := range findings {
				if f.Pattern == c.pattern {
					t.Errorf("pattern %q must NOT be flagged (%s); got finding: %s", c.pattern, c.why, f)
				}
			}
		})
	}
}

// TestDeadGrantLint_EngineParityByClass pins the lint's verdict for every
// evaluate() grammar class in templates/core/.opencode/plugins/
// shell-guard-core.js, per the PER-CLASS PARITY TABLE in dead_grant_lint.go
// (enumerated from the current engine file — never from memory). The earlier
// fixtures cover the mirrored core classes and the static-inspection
// carve-outs; this table covers the classes they miss. Every
// under-approximation row (caveats d/e/f and the caveat-c pattern-side rows)
// asserts NOT-flagged — the miss is advisory-only (B3): the lint fails silent
// on engine denies it does not model, never false-loud. No fixture outcome
// here is (or may become) a gate.
func TestDeadGrantLint_EngineParityByClass(t *testing.T) {
	cases := []struct {
		pattern     string
		wantDead    bool
		wantClass   string
		branch      string // evaluate() branch (parity-table row)
		disposition string
	}{
		// 05 harness-git deny — MIRRORED (anchor; deeper forms in Fixtures).
		{pattern: "vh-agent-harness git status *", wantDead: true, wantClass: DenyClassHarnessGit,
			branch: "05 vh-agent-harness git deny", disposition: "mirrored"},
		// 06 gate-wrapper deny — MIRRORED (anchor; carve-outs in Fixtures).
		{pattern: "vh-agent-harness exec .opencode/scripts/commit-gate.sh acquire *", wantDead: true, wantClass: DenyClassGateWrapper,
			branch: "06 gate-wrapper deny", disposition: "mirrored"},
		// 06 over-carve-out breadth — the row-06 "known breadth" documented
		// in dead_grant_lint.go, machine-pinned here: the engine grammar
		// admits exactly ONE operand after `bash -n` (ending in
		// commit-gate.sh, command ends there — isStaticGateInspection-
		// InDevShExec), so an extra operand falls OUT of the engine's
		// carve-out and the gate-wrapper deny fires; the lint's
		// isStaticGateInspectionShape carves out ANY `exec bash -n` prefix.
		// The divergence points fail-open (more carve-out = fewer flags —
		// a missed advisory, never a false positive), and tightening the
		// recognizer is a pinned non-goal. This row trips if that breadth
		// is ever narrowed silently.
		{pattern: "vh-agent-harness exec bash -n .opencode/scripts/commit-gate.sh extra-arg", wantDead: false,
			branch: "06 gate-wrapper deny (over-carve-out breadth)", disposition: "fail-open breadth: engine requires exactly one operand after bash -n and denies the excess; lint carves out the whole prefix — must stay not-flagged"},
		// 03 forbidden-pattern scan overlap: `apt install` hits the
		// apt-install-ad-hoc forbidden pattern (the engine's first-firing
		// deny) AND fails the allowlist — flagged with the non-allowlist
		// class; the dead verdict is identical either way.
		{pattern: "apt install foo *", wantDead: true, wantClass: DenyClassNonAllowlisted,
			branch: "03 forbidden-pattern scan (+final deny)", disposition: "mirrored-dead; class label names the structural non-membership (caveat c overlap)"},
		// 18 final non-git deny — MIRRORED anchor for a plain non-allowlisted
		// verb with no forbidden-pattern overlap.
		{pattern: "column *", wantDead: true, wantClass: DenyClassNonAllowlisted,
			branch: "18 final non-git deny", disposition: "mirrored (no forbidden-pattern overlap; the deny is the final branch)"},
		// 02 RF-B file-authoring deny — under-approximated (caveat c): the
		// redirection-carrying pattern intersects `echo *` and reads as
		// reachable while the engine denies every match.
		{pattern: "echo hi > out.md", wantDead: false,
			branch: "02 RF-B file-authoring deny", disposition: "under-approximated (caveat c): must stay not-flagged"},
		// 04 env-prefix-before-exec deny — under-approximated (caveat d).
		{pattern: "FOO=1 vh-agent-harness exec *", wantDead: false,
			branch: "04 env-prefix-before-exec deny", disposition: "under-approximated (caveat d): engine denies every match; lint reads reachable"},
		// 04 contrast rows: the deny's prefix shape is EXACTLY
		// `vh-agent-harness exec` — env-prefixed exec-ro / other self-forms
		// are genuinely engine-reachable.
		{pattern: "FOO=1 vh-agent-harness exec-ro *", wantDead: false,
			branch: "04 env-prefix-before-exec deny (prefix-shape contrast)", disposition: "engine-reachable: exec-ro is outside the deny prefix"},
		{pattern: "FOO=1 vh-agent-harness update *", wantDead: false,
			branch: "04 env-prefix-before-exec deny (prefix-shape contrast)", disposition: "engine-reachable: non-exec self-form is outside the deny prefix"},
		// 07 F1 wrapped-git-mutation deny — under-approximated (caveat e).
		{pattern: "vh-agent-harness exec git push *", wantDead: false,
			branch: "07 wrapped-git-mutation deny", disposition: "under-approximated (caveat e): engine denies every match; lint reads reachable"},
		{pattern: "vh-agent-harness exec --workdir=x git commit *", wantDead: false,
			branch: "07 wrapped-git-mutation deny (wrapper-flag form)", disposition: "under-approximated (caveat e): wrapper flags precede the payload; lint reads reachable"},
		// 07 contrast: a NON-mutation wrapped verb passes the engine's
		// mutation-only guard and hits the harness auto-allow — reachable.
		{pattern: "vh-agent-harness exec git log *", wantDead: false,
			branch: "07 wrapped-git-mutation deny (mutation-only contrast)", disposition: "engine-reachable: mutation guard plugs the mutation hole only"},
		// 11 git walker relative-`-C` deny — under-approximated (caveat f).
		{pattern: "git -C sub diff *", wantDead: false,
			branch: "11 git walker relative -C deny", disposition: "under-approximated (caveat f): engine denies every match; git umbrella reads table-rescuable"},
		// 11 contrast: an absolute `-C` payload is not walker-denied — the
		// allowlist sees the original tokens and the git ask branch keeps the
		// grant table-rescuable.
		{pattern: "git -C /abs/repo diff *", wantDead: false,
			branch: "11 git walker relative -C deny (absolute contrast)", disposition: "engine-reachable: absolute -C prompts (ask), table-rescuable"},
		// 12 git walker infoOnly auto-allow — reachable.
		{pattern: "git --help *", wantDead: false,
			branch: "12 git walker infoOnly auto-allow", disposition: "mirrored-as-reachable (git umbrella, caveat b)"},
		// 13 git walker mutation-slip deny (verb past global flags) —
		// under-approximated under the caveat (b)/(c) umbrella.
		{pattern: "git --no-pager push *", wantDead: false,
			branch: "13 git walker mutation-slip deny", disposition: "under-approximated (caveat b/c): mutation verbs out of scope"},
		// 14 git walker fullyStrippable — the stripped form matches
		// `git diff *`; genuinely reachable.
		{pattern: "git --no-pager diff *", wantDead: false,
			branch: "14 git walker fullyStrippable strip", disposition: "mirrored-as-reachable (git umbrella, caveat b)"},
		// Rows 15 (original-token pass → ask), 16 (allowlist pass), and 17
		// (git ask pass-through) have no dedicated rows here: they are the
		// Fixtures table's core coverage (git describe/diff/ls/grep/jq rows).
	}

	const fixtureAgent = "parity-agent"
	for _, c := range cases {
		c := c
		t.Run(c.branch+": "+c.pattern, func(t *testing.T) {
			r := LocationRule{Wildcard: Deny, Readonly: Allow, GitReadonly: Allow, HasGate: false, HarnessPolicy: HarnessPolicyDeny,
				ExtraBash: []BashEntry{{Pattern: c.pattern, Decision: Allow}}}
			findings := LintDeadGrantsForAgent(map[string]LocationRule{fixtureAgent: r}, fixtureAgent)
			if c.wantDead {
				found := false
				for _, f := range findings {
					if f.Pattern == c.pattern {
						found = true
						if f.DenyClass != c.wantClass {
							t.Errorf("deny class = %q; want %q (%s)", f.DenyClass, c.wantClass, c.disposition)
						}
					}
				}
				if !found {
					t.Fatalf("pattern %q must be flagged dead [%s — %s]; got findings %v", c.pattern, c.branch, c.disposition, findings)
				}
				return
			}
			for _, f := range findings {
				if f.Pattern == c.pattern {
					t.Errorf("pattern %q must NOT be flagged [%s — %s]; got finding: %s", c.pattern, c.branch, c.disposition, f)
				}
			}
		})
	}

	// 01 empty-command guard — mirrored-by-skip at the pattern grammar level:
	// the "*" wildcard and empty/whitespace pattern keys are skipped by the
	// config view (lintBashEntries), matching the engine's N/A grant-side row.
	t.Run("01 empty/null guard: wildcard and empty patterns skipped", func(t *testing.T) {
		cfg := []byte(`{
  "permission": { "bash": { "*": "allow", "": "ask", "  ": "allow" } }
}`)
		findings, err := LintDeadGrantsInConfig(cfg)
		if err != nil {
			t.Fatalf("LintDeadGrantsInConfig: %v", err)
		}
		if len(findings) != 0 {
			t.Fatalf("wildcard/empty/whitespace pattern keys must be skipped (mirrored-by-skip); got %v", findings)
		}
	})
}

// TestDeadGrantLint_BareSed pins the deliberate false-positive-avoidance
// choice for an exact bare grant of a MULTI-token readonly pattern's verb:
// engine patterns admit zero-extra-token matches for their OWN tokens only,
// so bare "sed" matches no engine pattern ("sed -n *" needs the -n token)
// and the exact grant "sed": allow IS dead. The lint flags it — correctly,
// because the engine's allowlist genuinely denies bare `sed`.
func TestDeadGrantLint_BareSed(t *testing.T) {
	const fixtureAgent = "fixture-agent"
	r := LocationRule{Wildcard: Deny, Readonly: Allow, GitReadonly: Allow, HasGate: false, HarnessPolicy: HarnessPolicyDeny,
		ExtraBash: []BashEntry{{Pattern: "sed", Decision: Allow}}}
	findings := LintDeadGrantsForAgent(map[string]LocationRule{fixtureAgent: r}, fixtureAgent)
	if len(findings) != 1 || findings[0].Pattern != "sed" || findings[0].DenyClass != DenyClassNonAllowlisted {
		t.Fatalf(`exact grant "sed" is engine-denied (no allowlist pattern matches bare sed) and must be flagged; got %v`, findings)
	}
}

// emittedFixtureConfig is a minimal emitted-shape opencode.jsonc carrying a
// transform-contributed dead allow (build: "pytest *"), a dead ask
// (researcher: "make *"), a dead deny (default: "docker *"), and a live
// rescuable git grant (build: "git describe *") plus canonical entries.
const emittedFixtureConfig = `{
    "agent": {
        "build": {
            "permission": {
                "bash": {
                    "*": "ask",
                    "git diff *": "allow",
                    "git describe *": "allow",
                    "ls *": "allow",
                    "pytest *": "allow",
                    "vh-agent-harness *": "allow"
                }
            },
            "prompt": "build"
        },
        "researcher": {
            "permission": {
                "bash": {
                    "*": "deny",
                    "make *": "ask",
                    "vh-agent-harness *": "deny"
                }
            },
            "prompt": "researcher"
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

// TestDeadGrantLint_InConfig drives the config-bytes view: agent attribution
// (default vs named agents), the three value classes, deterministic
// (agent, pattern) ordering, and SourceLine resolution.
func TestDeadGrantLint_InConfig(t *testing.T) {
	findings, err := LintDeadGrantsInConfig([]byte(emittedFixtureConfig))
	if err != nil {
		t.Fatalf("LintDeadGrantsInConfig: %v", err)
	}
	var got []string
	for _, f := range findings {
		got = append(got, f.Agent+"|"+f.Pattern+"|"+f.EntryValue+"|"+f.DenyClass)
	}
	// The fixture's dead entries only: the git describe / readonly / harness
	// entries are reachable and must NOT appear.
	expected := []string{
		`build|pytest *|allow|` + DenyClassNonAllowlisted,
		`default|docker *|deny|` + DenyClassNonAllowlisted,
		`researcher|make *|ask|` + DenyClassNonAllowlisted,
	}
	if len(got) != len(expected) {
		t.Fatalf("findings = %v; want exactly %d (dead allow + dead deny + dead ask; the git describe / readonly / harness entries must NOT appear)", got, len(expected))
	}
	for i := range expected {
		if got[i] != expected[i] {
			t.Errorf("finding[%d] = %q; want %q", i, got[i], expected[i])
		}
	}
	// SourceLine must resolve to the 1-based line of the pattern key.
	for _, f := range findings {
		if f.SourceLine <= 0 {
			t.Errorf("finding %v must carry a positive SourceLine in the config view", f)
			continue
		}
		lines := strings.Split(emittedFixtureConfig, "\n")
		if f.SourceLine > len(lines) || !strings.Contains(lines[f.SourceLine-1], `"`+f.Pattern+`":`) {
			t.Errorf("finding %v SourceLine=%d does not point at the pattern key line %q", f, f.SourceLine, lines[f.SourceLine-1])
		}
	}
}

// TestDeadGrantLint_InConfigCleanParseError covers the parse-failure path.
func TestDeadGrantLint_InConfigCleanParseError(t *testing.T) {
	if _, err := LintDeadGrantsInConfig([]byte("{not json")); err == nil {
		t.Fatalf("LintDeadGrantsInConfig must return the parse error for malformed config bytes")
	}
}

// TestDeadGrantLint_WordingContract enforces the operator-confirmed
// remediation-text contract (2026-08-28) as NEGATIVE assertions over every
// message surface this lint produces: no engine-allowlist-addition suggestion
// (no overlay/allowlist/allowed-commands/forbidden-patterns/permission-pack
// pointers) and no active-agent attribution (the findings name the table that
// CONFIGURES the grant, never an executing agent).
func TestDeadGrantLint_WordingContract(t *testing.T) {
	const fixtureAgent = "fixture-agent"
	r := LocationRule{Wildcard: Deny, Readonly: Allow, GitReadonly: Allow, HasGate: false, HarnessPolicy: HarnessPolicyDeny,
		ExtraBash: []BashEntry{
			{Pattern: "pytest *", Decision: Allow},
			{Pattern: "vh-agent-harness git *", Decision: Ask},
			{Pattern: "vh-agent-harness exec .opencode/scripts/commit-gate.sh *", Decision: Allow},
		}}
	findings := LintDeadGrantsForAgent(map[string]LocationRule{fixtureAgent: r}, fixtureAgent)
	if len(findings) != 3 {
		t.Fatalf("fixture must yield 3 findings; got %v", findings)
	}
	surfaces := []string{DeadGrantRemediation}
	for _, f := range findings {
		surfaces = append(surfaces, f.String(), f.Reason)
	}
	forbidden := []string{
		// allowlist-addition suggestions (there is no allow-side project seam)
		"overlay", "allowlist", "allowed-commands", "forbidden-patterns", "permission-pack",
		// active-agent attribution (session-derived identity)
		"current agent", "active agent", "was denied by", "attempted", "session",
	}
	for _, surface := range surfaces {
		lower := strings.ToLower(surface)
		for _, bad := range forbidden {
			if strings.Contains(lower, bad) {
				t.Errorf("wording contract violation: surface %q contains forbidden token %q", surface, bad)
			}
		}
	}
	// The remediation must name the three sanctioned paths.
	for _, want := range []string{"remove the grant", "downgrade the grant", "vh-agent-harness exec"} {
		if !strings.Contains(DeadGrantRemediation, want) {
			t.Errorf("DeadGrantRemediation must name %q; got %q", want, DeadGrantRemediation)
		}
	}
}
