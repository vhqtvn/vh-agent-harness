// Package permission defines the safety gate that runs BEFORE the runtime
// backend on exec/shell.
//
//   - Action { Allow, Deny, Ask } is the gate verdict.
//   - Hook.Evaluate(ctx, cmd) returns an Action + human reason + error.
//   - The WIRED DEFAULT is ShellGuardHook (slice 4b, shipped): a real
//     node-bridge that delegates each command to the shell-guard plugin
//     (node .opencode/plugins/shell-guard/eval.js) and NEVER returns Allow on
//     an engine fault (see shellguard_hook.go). It is constructed lazily via
//     activeHook(harnessRoot) in internal/cli/runtime_common.go.
//   - NoOpHook is the slice-4a fail-loud stub retained as the explicit Allow
//     test seam: it always returns Allow and logs a loud stderr warning on
//     EVERY evaluation that no command checking is active. It is NOT the wired
//     default — runtimeCmdDeps.hook == nil means "use the lazy ShellGuardHook
//     default," not NoOpHook.
package permission
