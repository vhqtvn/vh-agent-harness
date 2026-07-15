---
type: source
date: 2026-07-16
scope: CLI command surface — tree, doc drift, grouping, exec family, drift gate
status: research-complete, planner-ready (Slice 2)
---

# CLI Command Surface — Deep Study (source packet)

## Authoritative command tree (26 total)
Registration order = root.go:78-121 = --help order. rootCmd.SetHelpCommand (root.go:118);
hidden __exec_sandbox_child (root.go:121). overlay non-runnable parent -> overlay new (overlay_new.go:60), overlay docs (overlay_docs.go:35).
Status: P=primary A=advanced O=operator H=hidden.

| # | Command | File:Use | Status | README.md table | root.go Long | README.agent.md |
|---|---------|----------|--------|-----------------|--------------|-----------------|
|1|install|install.go:46|P|yes|yes|yes|
|2|update|update.go:30|P|yes|yes|yes|
|3|uninstall|uninstall.go:28|P|yes|NO|no|
|4|guide|guide.go:46|P|yes|yes|yes|
|5|example|example.go:21|A|NO|yes|yes|
|6|docs|docs.go:31|A|NO|yes|yes|
|7|sys-prompt|sys_prompt.go:31|A|NO|yes|yes|
|8|overlay(+new/docs)|overlay_new.go:46|A|NO|NO|yes|
|9|self-update|selfupdate.go:48|P|yes|yes|yes|
|10|version|version.go:34|P|yes|NO|no|
|11|preflight|preflight.go:33|A/health|yes|NO|no|
|12|doctor|doctor.go:45|A/health|yes|yes|yes|
|13|proposals|proposals.go:23|A/health|yes|NO|yes|
|14|diff|diff.go:21|A|yes|yes|yes|
|15|diagnostics-export|diagnostics.go:889|O|NO|yes|yes|
|16|exec|exec_shell.go:27|P/runtime|yes|yes|yes|
|17|exec-ro|exec_ro.go:29|A/runtime|NO|NO|yes(L345-409)|
|18|exec-sandbox|exec_sandbox.go:34|A/runtime|NO|NO|yes(L410-507)|
|19|shell|exec_shell.go:76|P/runtime|yes|yes|no|
|20|up|up_down.go:16|A/runtime|yes|yes|no|
|21|down|up_down.go:31|A/runtime|yes|yes|no|
|22|logs|logs_ps.go:18|A/runtime|yes|yes|no|
|23|status|status.go:17|P/status|yes|yes|no|
|24|ps|logs_ps.go:37|A/status|yes|yes|no|
|25|help(+migrate)|help_migrate.go:32|P|yes|yes|no|
|26|__exec_sandbox_child|exec_sandbox_child.go:16|H|n/a|n/a|n/a|

## Drift made precise
- README.md table (D5) omits 7: example, docs, sys-prompt, overlay, diagnostics-export, exec-ro, exec-sandbox.
- root.go Long-help (D6) omits ~8: uninstall, version, preflight, proposals, overlay, exec-ro, exec-sandbox, base help.
- Both miss: overlay, exec-ro, exec-sandbox.
- README.agent.md is the comprehensive doc; README.md (human-facing) is the lagging artifact.

## D3 — seam
seam is the internal render/apply substrate pipeline (classify->plan->per-class apply->lineage),
seam.go/seam_inventory.go/seam_drift.go, NO cobra.Command. Leaked into user vocab via
Short/Long strings (install.go:48, update.go:32, doctor.go:24,47, root.go:35). 70 .go + 29 .md hits.
Recommendation: glossary callout "seam = internal pipeline, not a command" in root.go Long + README + AGENTS;
lightly soften 3-4 prominent Short strings; do NOT rename code symbols. Domain-free vocab, safe in templates/core.

## D7/#8 — exec family
KEEP distinct verbs; do NOT unify to exec --mode=. Decisive: opencode permission matching
is VERB-based (vh-agent-harness exec-ro *: allow = never prompts). Unification either collapses
the allowlist distinction (breaking exec-ro prompt-free guarantee) or requires opencode to parse
flags (it doesn't). Three disjoint security planes:
- exec -> permission gate (shell-guard JS bridge, MAY prompt), dispatches through backend.
- exec-ro -> host-side Go intent classifier, hard-deny, allowlisted, dispatches through backend.
- exec-sandbox -> kernel Landlock+seccomp, HOST-LOCAL only, never reaches backend.
SetInterspersed(false) on all three -> a --mode flag collides with passthrough (second argument against).
No redundancy/dead code among the 5 exec files. exec_git_guard.go defines NO command (helper denyExecGitMutationPayload L38).
Do instead: cobra group runtime + exec-family paragraph in root.go Long -> README.agent.md section exec-family (already excellent L325-507).

## #11 — cobra grouping
cobra v1.10.2 (go.mod) supports AddGroup (since v1.7.0); unused today.
Scheme: Lifecycle (install/update/uninstall/self-update/overlay) . Orientation (guide/example/docs/sys-prompt/help) . Health/diagnostics (preflight/doctor/proposals/diff/diagnostics-export/status/version) . Runtime (exec/exec-ro/exec-sandbox/shell/up/down/logs/ps). Native cobra, no custom template.

## #13/#22 — automated drift gate (HIGHEST VALUE)
Reuse executeCapture(t, args) (root_help_test.go:24). New file internal/cli/command_surface_test.go:
- TestCommandSurface_HelpListsAllRegisteredCommands: iterate rootCmd.Commands() (skip Hidden),
  assert each Name() in no-args help output (catches D6; auto-extends, zero maintenance).
- README.md table: assert backtick-quoted cmds subset of registered (fictional-entry guard) AND
  registered-primary subset of table (omission guard for D5/#20). Advanced cmds optional in human table.
Files touched: new internal/cli/command_surface_test.go; reads ../../README.md.

## Coalesced Slice 2 — "CLI surface-consistency" (TDD-ordered)
1. FIRST: land drift-gate test — FAILS red on current state, mechanically documents drift.
2. Reconcile to green: cobra grouping (#11), root.go Long (#5/D6), README.md table (D5/#20), exec-family index (#8/D7), seam glossary (#7/D3).
3. Verify: gate green; --help renders groups; go test ./... + gofmt + go vet pass.
Docs+test only; no command-semantic change. README.md command table is OWNED by this slice.

## Verification claims
- 26 commands (25 visible + 1 hidden): root.go:78-121 + per-file Use:/Hidden:
- cobra v1.10.2 supports AddGroup: go.mod; groups landed v1.7.0
- no AddGroup/SetHelpTemplate usage: rg internal/cli/
- seam.go has no cobra.Command: rg "cobra.Command" internal/cli/seam.go
- README.md table omits exactly 7: README.md:107-116 vs root.go:78-121
- no existing surface-drift test: read root_help/seam_cli/seam_orphan/harness_dogfood_render _test.go
- exec_git_guard.go defines no command: rg "cobra.Command" exec_git_guard.go

## Promotion targets
new internal/cli/command_surface_test.go, internal/cli/root.go (Long + AddGroup), README.md,
optional templates/core/ glossary, README.agent.md (exec-family cross-link — already accurate),
AGENTS.md "Command hygiene" add exec-sandbox mention.

## Note
AGENTS.md "Command hygiene" documents exec and exec-ro but NOT exec-sandbox (gap; exec-sandbox is user-facing).
