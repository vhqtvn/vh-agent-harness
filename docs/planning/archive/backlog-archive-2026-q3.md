# Backlog Archive: 2026 Q3

This file stores older `done` and `cancelled` rows moved out of `docs/planning/backlog.md` by `.opencode/scripts/normalize-backlog.js` so the main backlog can stay focused on active work.

## Done

| ID | Status | Area | Task | Owner | Notes | Links |
| --- | --- | --- | --- | --- | --- | --- |
| P2-RESOLVER-007 | done | resolver | Cleanup/hygiene slice (residual polish after capability-installer + release pack committed): dead-code removal, double-call caching, stale fixtures, grammar/comment fixes, end-to-end + byte-compare test coverage, field-contract docs, deprecation cleanup. Low-risk, NO behavior change to rendered output. | build | 2026-07-01 DONE. (1) `internal/resolver/merge.go` — deleted dead `resolveShadowing` (zero production callers after `ResolveContributions` export); rewrote stale cross-reference doc comment. (2) `internal/cli/profile.go` — cached double `activeOverlays(target)` call in `resolveCapabilityAnswers` into one local; F4 grammar fix in `modulesDeprecationWarning` (pluralize entry/entries for N>1); updated `liveProfileModules` comment (embedded default no longer ships `modules:`). (3) `internal/overlay/overlay_test.go` — F3 stale fixture `knownPackNames` `[]string(nil)`→`["release"]` + comment block. (4) `internal/cli/seam.go` — F5 softened `emitModulesDeprecationWarning` comment to state real fire surface (every renderSeamStaging path). (5) NEW tests: Case-1 end-to-end `TestDiscoverPackContributions_ManifestProjectPackReplacesEmbedded` (project pack WITH manifest replaces embedded; companion to Case-2 no-manifest test) + `TestSeamRender_ReleasePathsByteConverge` (byte-compare opencode.jsonc + agent roster across capabilities vs overlays selection paths). (6) `README.agent.md` — F3 field-contract subsection (profile enum / capabilities union / overlays override / modules deprecated). (7) `templates/overlays/release/agents/releaser.md` — F5 wrapper is project-supplied note. (8) F2 `templates/core/.vh-agent-harness/vh-harness-profile.yml` — removed vestigial `modules: [core]` block (domain-neutral, verified via git diff). Test delta: +2 `func Test` (baseline 472 → 474); item 4 updated 1 existing test case. Verify: `go build ./...`, `go vet ./...`, `gofmt -l .` (empty), `go test ./...` all pass; `vh-agent-harness doctor` HEALTHY. `templates/core/` domain-free invariant held. |  |

## Cancelled

| ID | Status | Area | Task | Owner | Notes | Links |
| --- | --- | --- | --- | --- | --- | --- |
