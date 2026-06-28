# Backlog Archive: 2026 Q2

This file stores older `done` and `cancelled` rows moved out of `docs/planning/backlog.md` by `.opencode/scripts/normalize-backlog.js` so the main backlog can stay focused on active work.

## Done

| ID | Status | Area | Task | Owner | Notes | Links |
| --- | --- | --- | --- | --- | --- | --- |
| P1-HARDEN-001 | done | hardening | Adoption-hardening Slice 1: trim inert example config fields (W1) + contract test (Q3), empty-token warning (W3), re-seed workaround docs (W6), remove phantom runtime-audit (W8), document PROJECT_SLUG case rule (W10), contributor principle + template-lint (W5/Q4), fail-closed malformed overlays (W9/Q5). Heavy lineage work (W2/W7) + health/audit command (W4) deferred to Slice 2. | build | 2026-06-28 DONE. Closes W1,W3,W5,W6,W8,W9,W10 (+Q3/Q4/Q5). Changed files: templates/examples/.vh-agent-harness/project.config.json (T1: keep only 4 consumed fields); internal/cli/projectconfig_test.go (T2: contract test asserting example advertises exactly the 4 consumed fields); internal/cli/projectconfig.go + install.go + update.go + guide.go (T3: warnEmptyProjectConfigTokens loud stderr warning when config absent/token empty, fires under install/update/dry-run/guide); README.agent.md + internal/cli/guide.go (T4: re-seed `rm <file> && update` docs + nextSteps); templates/core/.vh-agent-harness/AGENTS.core.md (T5: remove phantom runtime-audit from command-template list; rendered AGENTS.md + .vh-agent-harness/AGENTS.core.md regenerated via make update, runtime-audit gone repo-wide except this backlog row); templates/core/.opencode/commands/harness.md (T6: document PROJECT_SLUG case rule — UPPER before `_`, lower otherwise); docs/ai/template-authoring.md + internal/substrate/templates_lint_test.go (T7: contributor principle "renderer is allowlist-tight, no comment-stripping" + lint test failing build on renderer-directive comments); internal/cli/seam.go + seam_cli_test.go (T8: fail-closed on overlay OpenPackFor error instead of warn-and-skip; + fixed stale web-overlay fixture that only passed via the old silent skip). Decision (T8): no distinct discovered-overlay category exists (activeOverlays reads profile overlays: exclusively) → fail-closing ALL overlays is correct. Verify: go test ./... pass; go vet ./... clean; gofmt -l . empty; make update ran (143 files); new tests (T2 contract, T7 lint) pass + assert; update --dry-run warning confirmed on stderr; README.agent.md consistent. Commit pending via committer subagent. |  |
| P1-CLI-010 | done | cli | Seam-aware diff/uninstall/preflight (work on seam installs, not just legacy manifest). |  | 2026-06-27 v0.1.1 |  |
| P1-CLI-011 | done | cli | exec: run mutating commands (fix shell-guard double-gate); make `--` optional. |  | 2026-06-27 v0.1.2 |  |

## Cancelled

| ID | Status | Area | Task | Owner | Notes | Links |
| --- | --- | --- | --- | --- | --- | --- |
