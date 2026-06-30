
## release (releaser)

- **releaser** — release specialist (thin spine + default tag-driven adapter).
  Leaf specialist; accepts no downstream delegations (`task: {"*":"deny"}`).

### Inbound

- `build` → `releaser`
- `coordination` → `releaser`
- `project-coordinator` → `releaser`

(`delegateFrom: [build, coordination, project-coordinator]` — declared in this
pack's permission-pack.jsonc; the Go-native emitter injects the matching
`releaser: allow` edge into each declaring orchestrator's permission.task map.)

### Outbound

None. The releaser does **not** delegate to the committer. The `core/gated-commit`
hard dependency is a PREREQUISITE cluster (a release presupposes a clean,
reviewed commit history produced through the gated-commit protocol), not a
delegation target from this agent.

### Commit-gate separation

The releaser is NOT part of the gated-commit protocol and carries `gate: deny`.
Its sole mutation surface is a single release-tag invocation through the
project's sanctioned release-tag wrapper (`vh-agent-harness exec <wrapper>`),
which performs the actual `git tag -a` and optional `git push`. Raw
`git tag` / `git push` / `git add` / `git commit` are forbidden to every agent
by the shell-guard `git-mutation-bypass` rule; the wrapper is the only path.
