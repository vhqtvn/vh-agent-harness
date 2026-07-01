
## harness-dogfood (harness-release-readiness)

- **harness-release-readiness** — read-only release-readiness reporter for THIS
  repo (`vh-agent-harness`); orchestrator ABOVE the existing `releaser`. Leaf
  reporter; accepts no downstream delegations (`task: {"*":"deny"}`). Performs no
  mutation (read-only evidence-gathering only; `gate: deny`, `devSh: deny`).

### Inbound

- `build` → `harness-release-readiness`
- `coordination` → `harness-release-readiness`
- `project-coordinator` → `harness-release-readiness`

(`delegateFrom: [build, coordination, project-coordinator]` — declared in this
pack's permission-pack.jsonc; the Go-native emitter injects the matching
`harness-release-readiness: allow` edge into each declaring orchestrator's
permission.task map.)

### Outbound — handoff

- `harness-release-readiness` → `releaser` (HANDOFF, only when `ready: yes` AND
  a human explicitly approves). The readiness reporter does NOT create the tag;
  it populates its `handoff_to_releaser` field with `(version_hint, last_tag,
  commit_range)` and the human + `releaser` act on it. The `releaser` computes
  the authoritative next version from discovered history (its invariant #4: the
  readiness hint is advisory; conflicts cause it to refuse) and performs the
  single sanctioned release-tag wrapper invocation. This edge is NOT a task
  delegation from the readiness agent to the releaser — the readiness agent's
  `task: {"*":"deny"}` refuses all downstream delegations. The handoff flows
  through the report field + the human, not through OpenCode's task surface.

### Outbound — delegation

- `harness-release-readiness` → `docs-steward` (DELEGATION by report flag, not by
  task surface): G1 migration-note authorship and G3 docs-coverage gaps are
  flagged in the report's `delegated_owners` for `docs-steward` to act on. Like
  the releaser handoff, this is a report-driven handoff, not a direct task call —
  the readiness agent emits the report and stops.
- `harness-release-readiness` → `build` (DELEGATION by report flag): any code
  change a check surfaces (e.g. a runtime consumer warning for the Phase-5 roster
  shrink, explicitly OUT of this agent's scope to implement) is flagged in
  `delegated_owners` for `build`, never performed here.

### Commit-gate separation

The readiness reporter is NOT part of the gated-commit protocol and carries
`gate: deny`. It performs NO mutation of any kind: no tag, no commit, no push, no
file edit (its report is emitted only in its final response, never written to
disk). The `core/release` hard dependency (which transitively pulls `core/gated-commit`)
exists because a readiness handoff presupposes a clean, reviewed commit history
produced through the gated-commit protocol — it is a prerequisite cluster, not a
delegation target from this agent. Raw `git tag` / `git push` / `git add` /
`git commit` are forbidden to every agent by the shell-guard `git-mutation-bypass`
rule; the readiness reporter relies on that backstop and adds its own read-only
invariant on top.
