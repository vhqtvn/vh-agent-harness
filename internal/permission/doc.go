// Package permission defines the safety gate that runs BEFORE the runtime
// backend on exec/shell.
//
// Slice 4a ships ONLY the contract:
//
//   - Action { Allow, Deny, Ask } is the gate verdict.
//   - Hook.Evaluate(ctx, cmd) returns an Action + human reason + error.
//   - NoOpHook is a fail-loud stub: it always returns Allow, but logs a loud
//     stderr warning on EVERY evaluation that no command checking is active.
//
// Slice 4b replaces NoOpHook with a real node-bridge that delegates each
// command to the shell-guard plugin (node .opencode/plugins/shell-guard/eval.js).
// Until then the wired default is NoOpHook and the gap is loud, not silent.
package permission
