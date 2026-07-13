# release

An **embedded** overlay pack that ships the tag-driven `releaser` workflow — the
release specialist that computes the next semantic-version tag from a project's
conventional-commits history and applies it through the project's **sanctioned
release-tag wrapper** (never raw `git tag` / `git push`).

It is the reference implementation of the capability-installer overlay
integration and the first shipped embedded overlay pack.

## What this is

The pack contributes one agent — `releaser` — plus a `core/release` capability
manifest with a hard dependency on `core/gated-commit` (the releaser delegates
the migration-note commit to the gated-commit agents, so selecting this pack
pulls that cluster in automatically).

The agent is structured as a **thin spine + default adapter**:

- The **spine** owns the flow-control contract, the safety/refusal taxonomy, the
  commit-gate separation rule, and the JSON output schema. It is the part a
  project keeps verbatim.
- The **adapter** owns the four payload steps (discover / decide / prepare /
  execute). The default adapter shipped here is **tag-driven + conventional-
  commits**: it reads the last tag, walks commits-since-last-tag, applies the
  conventional-commits semver rule (`feat:` → minor, `fix:`/`perf:` → patch,
  `!:` / `BREAKING CHANGE:` → major), prepares the migration note, and applies
  the tag via the sanctioned wrapper.

## Selection

The pack is selected either way and the two paths converge:

- `capabilities: [core/release]` — the explicit capability opt-in, OR
- `overlays: [release]` — the expert-override pack selection.

Selecting `core/release` also pulls the `core/gated-commit` cluster in via the
hard-dep closure, so both selection paths render the same cluster.

## Invariants (spine contract)

1. **Never raw git mutation.** Mutations occur ONLY via (a) one narrow
   committer delegation for the migration note in Prepare, and (b) the
   sanctioned release-tag wrapper in Execute.
2. **Never raw-tag.** The annotated tag is applied ONLY through the sanctioned
   release-tag wrapper.
3. **Never create a tag you were not asked for.**
4. **Discovered state is authoritative; orchestrator hints are non-binding.**
5. **Order tags numerically, never lexically.**
6. **Refuse rather than guess.** Emit the refusal JSON shape and stop when any
   payload step cannot produce a confident answer.

## Configuration

The releaser agent has NO separate config file — it derives everything from the
repo state (git history, last tag, the sanctioned release-tag wrapper path).
The sanctioned release-tag wrapper is the one project-supplied surface the
releaser depends on; the agent discovers it from the repo.

For the full agent contract (the four-step payload flow, the JSON output schema,
the refusal taxonomy), read the rendered `releaser.md` after selecting the pack,
or inspect the agent source in this pack's `agents/releaser.md`.
