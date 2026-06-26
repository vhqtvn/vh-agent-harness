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

## Dogfood loop

`.vh-agent-harness/` holds this repo's own profile / run-shape / AGENTS sources;
`.opencode/` is the rendered corpus. Edit `templates/core/` (the source),
rebuild, then `vh-agent-harness update` (or `make update`) to regenerate.

## Runtime

Plain Go on the host — `run-shape.yml` uses `backend: host-shell`. Build/test
with `go build` / `go test` (or `make`). No container required.
