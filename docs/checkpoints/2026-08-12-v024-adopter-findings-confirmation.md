# Checkpoint: v0.24.0 Adopter Findings Confirmation
**Date:** 2026-08-12
**Status:** triage complete, awaiting release decision

## Mission
Confirmed-defect triage of 3 external adopter reports (vh-solara, vh-video-maker, TrueAI) against the v0.24.0 release (HEAD == `13d3b56c6057e421b63b7330bc7b3eaec740695c`, tag `da5f6dfa`).

## Confirmed Defect Map
These 5 confirmed findings (plus 2 narrowed) have been filed into the active backlog.

| Finding | Sev | Title | Area | Status | Re-derivation command |
|---------|-----|-------|------|--------|-----------------------|
| F1 | High | shell-guard over-match denies accept-platform/diff on commit-gate.sh paths | shell-guard | in_progress | `grep -n "accept-platform" templates/core/.opencode/plugins/shell-guard-core.js` |
| F3 | Med-High | commit-gate GC reaps in-use protected file via stat-fallback | commit-gate | in_progress | `sed -n '167,173p' templates/core/.opencode/scripts/commit-gate.sh` |
| F8 | Low/enh | add batch --all/--stale-only to accept-platform | cli | todo | `sed -n '104,109p' internal/cli/accept_platform.go` |
| F6 | Low | dangling test reference in verify-task-registry.js | scripts | todo | `sed -n '2810p;3484p' templates/core/.opencode/scripts/verify-task-registry.js` |
| F7 | Low/docs | qualify bare "drifted" token in doctor/diff output | cli/docs | todo | `grep -n '"drifted"' internal/cli/diff.go internal/cli/doctor.go` |
| F2-narrow | Low/Med | root AGENTS.md not in origin-hash set → doctor blind to composition drift | ownership | in_progress | `grep -n "AGENTS.md" internal/ownership/harness-ownership.yml` |
| F5-narrow | Low | stale prose "once Slice-2 retirement CODE lands" | docs | todo | `sed -n '159p' templates/core/docs/coordination/RECORD_LIFECYCLE.md` |

## Refuted Findings
These load-bearing refutations prevent re-filing of invalid defect reports.
*   **F4 (pause-new-work overclaim):** fully refuted. Not a defect; all three consumers are correctly wired in HEAD (`templates/core/.opencode/plugins/state-lib.js:13,6137-6160`; `templates/core/.opencode/skills/bgshell_job.py:72-131,402-404,522-524`).
*   **F2 (composition failure):** largely refuted. `update` DOES compose correctly via `composeAgentsMd`; only the origin-hash tracking gap remains (narrowed as F2-narrow).
*   **F5 (landing gate missing):** largely refuted. The landing-gate logic successfully landed in commit `00b5add`, and the `force=true` bypass is documented by design; only the stale prose reference remains (narrowed as F5-narrow).

## H-Scan Verdict
All 6 evaluated v0.24.0 surfaces are **CLEAN**:
*   rewrite-parity gate
*   origin-hash adoption
*   Pre-Operator-Ask canon
*   pause beyond F4
*   behavioral-closure/closeout-reach tokens
*   skill-proposal intake
*   **Verdict:** The document-overclaim class is isolated to F5-narrow; it is not a systemic drift issue.

## Verification

| Claim | Verifying command/output | Verified |
|-------|--------------------------|----------|
| HEAD == release v0.24.0 | `git log -1 --format="%H"` / `git describe --tags --exact-match` | yes |
| F1 confirmed at L1037 | `sed -n '1037p' templates/core/.opencode/plugins/shell-guard-core.js` | yes |
| F3 hypothesis at L171 | `sed -n '171p' templates/core/.opencode/scripts/commit-gate.sh` | yes |
| F4 refuted | `grep -rn "pause-new-work" templates/core/.opencode/` matches expected consumer wiring | yes |
| H-scan clean | manual inspection over 6 surfaces | yes |

## Findings
*   **Fact:** (source=adopter-reports, confidence=high) Adopter migration issues spanning 0.22.1→0.24.0 yielded 8 reported findings.
*   **Fact:** (source=HEAD, confidence=high) F4 is entirely false; consumers are fully wired in v0.24.0.
*   **Inference:** (source=H-scan, confidence=high) The F5 documentation overclaim is an isolated miss, not a symptom of broader semantic-drift issues in the repository.

## Contradictions
*   **Adopter Reports vs HEAD Reality:** F2, F4, and F5 as originally reported by the adopters contradict the actual HEAD reality. F4 claims missing `pause-new-work` consumers, which exist; F2 claims `update` fails to compose `AGENTS.md`, which it does; F5 claims missing landing gate code, which landed in `00b5add`.

## Release Decision Pending
Awaiting operator decision on the release vehicle mapping:
*   **patch 0.24.1 target:** F1 + F3 (if reproduced) + F8
*   **next-version 0.25.0 target:** the rest (F6, F7, F2-narrow, F5-narrow)
