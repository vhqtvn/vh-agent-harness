<!-- _pack-skeleton/callable-graph-snippet.md — optional pack routing doc -->
<!-- Copy this pack to .vh-agent-harness/overlays/<pack>/ and edit. This file is -->
<!-- appended to the rendered callable-graph when the pack is active. Replace -->
<!-- <pack> and <name>. Tokens like {{PROJECT_NAME}} resolve at render time. -->

## <pack> specialists

- **<name>** (scope): one line describing what this specialist does and its edit scope.

### <pack> routing

- `build`, `coordination`, and `project-coordinator` may delegate to `<name>` (task allow).
- `<name>` stays within its scope; cross-scope work is handed back.
- Commands surfaced by this pack: `<cmd-1>`, `<cmd-2>`.
