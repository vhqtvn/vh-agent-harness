
## release (releaser)

- **releaser** — release specialist (thin spine + default tag-driven adapter).
  Leaf specialist except UP TO TWO narrow single-path `committer` delegations
  — one for the migration note (`templates/migrations/v<next>.md`) and one for
  the manifest-only commit (`.vh-agent-harness/release-defer-dispositions.json`,
  manifest-authority mode only) — matching the releaser contract
  (`task: {"committer":"allow","*":"deny"}`).

### Inbound

- `build` → `releaser`
- `coordination` → `releaser`
- `project-coordinator` → `releaser`

(`delegateFrom: [build, coordination, project-coordinator]` — declared in this
pack's permission-pack.jsonc; the Go-native emitter injects the matching
`releaser: allow` edge into each declaring orchestrator's permission.task map.)

### Outbound

UP TO TWO narrow single-path delegations: `releaser` → `committer`, one for
the migration note (`templates/migrations/v<next>.md` — the release migration
note) and one for the manifest-only commit
(`.vh-agent-harness/release-defer-dispositions.json`, manifest-authority mode
only). Each delegation instructs the committer to use the canonical
gated-commit message-as-file protocol; the **committer** (not the releaser)
runs `commit-gate.sh`. No other outbound task delegation exists. The
release-tag wrapper invocation in Execute is a direct `vh-agent-harness exec`
call, not a task delegation. The `core/gated-commit` hard dependency is
therefore BOTH a prerequisite cluster (a release presupposes a clean,
reviewed commit history produced through the gated-commit protocol) AND the
delegation target for both committer delegations.

### Commit-gate separation

The releaser is NOT a gate caller: it does not invoke `commit-gate.sh` itself
and is a **gateExempt committer-delegator** (its permission-pack declares
`gateExempt: true` and OMITS the `gate` decision — no `gate` key in its
location). Its two sanctioned mutations are (a) UP TO TWO narrow single-path
task delegations to `committer` — one for the migration note
(`templates/migrations/v<next>.md`) and one for the manifest-only commit
(`.vh-agent-harness/release-defer-dispositions.json`, manifest-authority mode
only) — where the **committer** — not the releaser — runs the gated-commit
message-as-file protocol and independently holds the gate, and (b) the single
release-tag invocation through the project's sanctioned release-tag wrapper
(`vh-agent-harness exec <wrapper>`). The wrapper is tag-only: it performs the
actual `git tag -a` and optional `git push` and MUST NOT stage or commit the
migration note (that is the committer's job via the delegation). The releaser
itself carries NONE of the `commit-gate.sh` mutation subcommands
(acquire/commit/release/heartbeat/revert/stage-message) nor `uuidgen` in its
bash block — all omitted under gateExempt; the committer independently holds
the gate. gateExempt is WHY `gate` is omitted (not denied): OpenCode's
`deriveSubagentSessionPermission` merges parent denies into a subagent session
via findLast, so a parent `gate:deny` on the releaser would bleed into the
delegated committer session and override the committer's `gate:allow`, blocking
the very gated-commit commands the note commit runs through. Omitting the gate
decision keeps the releaser's posture out of the committer's session; it does
NOT make the releaser a gate caller. The releaser refuses if the wrapper/tag
mechanism is absent — there is no fallback to raw git. Raw `git tag` /
`git push` / `git add` / `git commit` are forbidden to every agent by the
shell-guard `git-mutation-bypass` rule; the wrapper is the only path for the
tag.
