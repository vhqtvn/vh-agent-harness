<!-- PROJECT mission — composed into AGENTS.md after AGENTS.core.md. -->

# vh-agent-harness — Mission & Project Rules

This repository builds **vh-agent-harness**: a single static Go binary that
installs, manages, and runs a repo-resident AI agent harness (OpenCode-first).
The repo **dogfoods itself** — this harness is installed here and used to
develop the harness.

## Architecture map

- `cmd/vh-agent-harness/` — entrypoint.
- `internal/` — substrate, ownership, schema, lineage, runshape, runtime, hooks,
  overlay, proposals, drift, permission, cli.
- `templates/core/` — the embedded corpus (`go:embed`). THIS is what ships into
  projects; it must stay **domain-free** (tokens only, no project specifics).
- `corpus.go` / `core_manifest.go` — embed roots + ownership classification.

## Non-negotiable rules

- **Keep `templates/core/` domain-free.** No brand/domain literals; use
  `{{PROJECT_NAME}}` / `{{PROJECT_SLUG}}` / `{{COORDINATOR_DIR}}` tokens. Project
  specifics belong in overlays, never in core.
- **The binary/command is `vh-agent-harness`**, never the generic `harness`. The
  concept word "harness" stays in prose; only the binary identity is full.
- **`go test ./...`, `gofmt`, and `go vet` must pass** before commit.
- **Ownership is the safety contract.** A plain render may only overwrite
  `platform_managed` (and active `overlay_extension`); everything else is
  preserved / seeded-once / schema-reconciled. Always preview with `--dry-run`.
- **Agent-operability is a feature.** `guide`, `--dry-run`, and the next-steps
  footers must stay accurate to the real command surface.
- **`README.agent.md` must always be up-to-date.** It is the agent operating
  manual; any change to the command surface, the configurable-file set
  (`vh-agent-harness example`), ownership, or the runtime/exec contract MUST be
  reflected there in the same change. Treat a stale `README.agent.md` as a bug.

## Dogfood loop

`.vh-agent-harness/` holds this repo's own profile / run-shape / AGENTS sources;
`.opencode/` is the rendered corpus. Edit `templates/core/` (the source),
rebuild, then `make update` to regenerate.

**Prefer `make update`, not a bare `vh-agent-harness update`.** `update` renders
only from the corpus embedded in the running binary, never from the working tree,
so the `Makefile` target builds first. In this dev/dogfood repo the on-disk
`templates/core/` is often newer than the PATH binary's baked-in copy; a bare
`vh-agent-harness update` will silently overwrite in-progress `.opencode/` edits
with those older bytes, and `doctor` does not detect the revert (it re-renders
from the same stale embedded corpus and byte-compares, reporting "in sync"). The
footgun is specific to a repo that edits `templates/core/`; consumers whose binary
embeds the same corpus they run with are unaffected.

This footgun is now ENFORCED, not just documented: a target that positively
looks like a source checkout (it carries BOTH `corpus.go` and `templates/core/`
at its root) is guarded. When the binary's embedded corpus DIFFERS from the
checkout's `templates/core/`:

- a LIVE `vh-agent-harness update` REFUSES (non-zero exit, before any write);
  the recovery is `make update` (the `Makefile` target rebuilds first), and
  `--allow-stale-corpus` is the explicit override;
- `vh-agent-harness update --dry-run` still previews but warns that the output
  reflects the BINARY's embedded corpus, not what a rebuilt binary would render;
- `vh-agent-harness doctor` surfaces a non-failing `dev-stale-embed` WARN and
  qualifies its `managed-drift` line ("in sync with the embedded corpus in this
  binary", never a bare "in sync"). The WARN alone never makes doctor UNHEALTHY.

A target without BOTH markers (every consumer) is completely unaffected — the
guard never fires there. The guard never asserts which side is "ahead" (a byte
comparison proves difference, not chronology); the recovery is `make update`
either way.

## Runtime

Plain Go on the host — `run-shape.yml` uses `backend: host-shell`. Build/test
with `go build` / `go test` (or `make`). No container required.

## Coordination cross-reference

Before interrupting the operator for a decision, follow the **Pre-Operator-Ask
Routing Canon** (`vh-agent-harness docs opencode-session-workflow`):
self-resolve avoidable asks (re-derive, investigate, act-if-reversible) first,
and ask the operator only for named protected decisions that survive that
triage. It is INFORMS-only and does not lower any `p0`/ownership/permission/
review/transition gate.
