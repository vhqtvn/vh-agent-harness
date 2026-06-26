# Claude / Cross-Agent Rules — vh-agent-harness

Use `AGENTS.md` as the primary source of truth. It is composed by the harness
from `.vh-agent-harness/AGENTS.core.md` + `.vh-agent-harness/AGENTS.mission.md`
— do not edit `AGENTS.md` by hand.

## Mandatory behavior

- Keep `templates/core/` **domain-free**; project specifics go in overlays.
- The binary/command is **`vh-agent-harness`** (never the generic `harness`).
- `go test ./...` + `gofmt` + `go vet` must pass before commit.
- Ownership classes are the safety contract; preview seam changes with
  `vh-agent-harness install/update --dry-run`.
- Run repo commands through `make` / `go`; keep scratch under `./tmp/`.
