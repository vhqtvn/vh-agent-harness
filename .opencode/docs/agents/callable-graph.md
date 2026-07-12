# Callable Graph

## Public entrypoints

Only these should be treated as direct user-facing agents:

- `coordination` (read-only routing, default primary agent)
- `build` (execution owner, delegated by coordination)

All other agents are delegated specialists.

## Routing model

1. `coordination` routes to `build` by default.
2. `coordination` may directly call read-only specialists when scope is narrow.
3. `build` owns implementation and may call editable specialists.
4. Closeout goes through `commit-message` and/or `ship-review` as needed.

## Delegation ownership

Only these agents should fan out via `permission.task`:

- `build`
- `coordination`
- `project-coordinator`
- `commit-message` (to `commit-reviewer` only)
- `commit-reviewer` (to `commit-reviewer-a`, `commit-reviewer-b`, and `commit-reviewer-c` only; `commit-reviewer-d` deferred until premium tier is enabled)

All other specialists should keep `task: { "*": "deny" }` to prevent lateral drift.

## Specialist classes

This graph lists ONLY the CORE roster shipped by the harness. Overlay packs
(e.g. a web overlay, a project domain overlay, ...) append their own specialists
to this graph via a `callable-graph-snippet.md` that is merged onto this file
when the pack is selected in `vh-harness-profile.yml` `overlays: [...]`. Do not
hand-write overlay specialists here — declare them in the overlay pack's snippet.

- Read-only specialists (core):
  - `project-coordinator`
  - `debate`
  - `planner`
  - `researcher`
  - `repo-explorer`
  - `commit-reviewer`
  - `ship-review`
  - `solution-brief`
- Editable specialists (core):
  - `docs-steward`

## Internal cluster pattern

For private helper families (implemented for debate):

- one visible orchestrator (`debate`)
- hidden helpers (`debate-*`)
- strict task allowlist on the orchestrator:
   - `"task": { "*": "deny", "debate-*": "allow" }`

#### Commit-reviewer cluster

`commit-reviewer` is an internal cluster: one visible orchestrator (`commit-reviewer`) dispatches to hidden leaves across multiple tiers. Tier structure is defined in `.opencode/config/review-tiers.json` — currently Tier 1 (free, B+C), Tier 2 (cheap, A), and Tier 3 (premium, D, disabled). The leaves are identical except for description frontmatter; running independent reviews across tiers reduces single-model blind spots. The orchestrator performs mechanical JSON aggregation with strict consensus within each tier and fail-fast escalation across tiers — all tiers must approve for an overall approve. The delegation ownership rule (§2) applies: only the orchestrator may call the leaves via `task`.

Cluster pattern:
- visible: `commit-reviewer` (orchestrator, in read-only specialists list)
- hidden: `commit-reviewer-a`, `commit-reviewer-b`, `commit-reviewer-c` (leaves, not in callable graph; `commit-reviewer-d` deferred until premium tier enabled)
- task allowlist on orchestrator: `{ "*": "deny", "commit-reviewer-a": "allow", "commit-reviewer-b": "allow", "commit-reviewer-c": "allow" }`
- leaves have `task: { "*": "deny" }` — cannot call anyone
- review modes are documented in `commit-reviewer-modes.md`

## Research-to-debate workflow

For web-grounded option discovery or creative solution finding:

- keep retrieval and source gathering in `researcher`
- hand off grounded options to `debate` for bounded comparison and critique
- do not add a second hidden web-research path under `debate-*` unless the
  callable graph is intentionally revised

## Naming consistency rule

Agent IDs must match across:

- `opencode.jsonc`
- `.opencode/agents/*.md`
- `AGENTS.md`
- `docs/coordination/*` lane and role docs

Do not keep dual IDs for one role (for example a release agent carrying both a
generic name and a project-specific name).



## harness-dogfood (harness-release-readiness)

- **harness-release-readiness** — read-only release-readiness reporter for THIS
  repo (`vh-agent-harness`); orchestrator ABOVE the existing `releaser`. Leaf
  reporter; accepts no downstream delegations (`task: {"*":"deny"}`). Performs no
  mutation (read-only evidence-gathering only; `gate: deny`, `harness: deny`).

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



## release (releaser)

- **releaser** — release specialist (thin spine + default tag-driven adapter).
  Leaf specialist except ONE narrow `committer` delegation for the migration
  note (`task: {"committer":"allow","*":"deny"}`).

### Inbound

- `build` → `releaser`
- `coordination` → `releaser`
- `project-coordinator` → `releaser`

(`delegateFrom: [build, coordination, project-coordinator]` — declared in this
pack's permission-pack.jsonc; the Go-native emitter injects the matching
`releaser: allow` edge into each declaring orchestrator's permission.task map.)

### Outbound

ONE narrow delegation: `releaser` → `committer`, for exactly one file
(`templates/migrations/v<next>.md` — the release migration note). The
delegation instructs the committer to use the canonical gated-commit
message-as-file protocol; the **committer** (not the releaser) runs
`commit-gate.sh`. No other outbound task delegation exists. The release-tag
wrapper invocation in Execute is a direct `vh-agent-harness exec` call, not a
task delegation. The `core/gated-commit` hard dependency is therefore BOTH a
prerequisite cluster (a release presupposes a clean, reviewed commit history
produced through the gated-commit protocol) AND the delegation target for the
migration-note commit.

### Commit-gate separation

The releaser is NOT a gate caller: it does not invoke `commit-gate.sh` itself
and is a **gateExempt committer-delegator** (its permission-pack declares
`gateExempt: true` and OMITS the `gate` decision — no `gate` key in its
location). Its two sanctioned mutations are (a) ONE narrow task delegation to
`committer` for the migration note (`templates/migrations/v<next>.md`), where
the **committer** — not the releaser — runs the gated-commit message-as-file
protocol and independently holds the gate, and (b) the single release-tag
invocation through the project's sanctioned release-tag wrapper
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
