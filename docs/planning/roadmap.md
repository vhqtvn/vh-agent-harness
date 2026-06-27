# vh-agent-harness — Roadmap

Phase ordering and milestone intent. Task-level status lives in
`docs/planning/backlog.md`; this file holds the higher-level arc.

## Phases

- **P1 — Ship an installable, agent-operable harness (current):** a single static
  binary that installs/updates/runs a repo-resident OpenCode harness through the
  substrate seam, dogfoods itself, and is safe to adopt into existing repos.
  v0.1.x is published; the current focus is making a *coding agent* able to
  operate and extend the harness from inside a consumer repo without external
  docs (overlay discoverability).
- **P2 — Extension ergonomics:** first-class overlay authoring (scaffolding +
  in-repo guidance), richer `guide`/`example` self-direction, and shipped example
  overlays for common needs (e.g. a release subagent).

## Milestones

- [x] **Installable + self-managing** — install/update/doctor/uninstall over the
  seam; lineage + ownership; dogfooded. (v0.1.0–v0.1.4)
- [x] **Consumer-safe commit gate** — validate-opencode-config.py accepts valid
  JSONC; no longer blocks commits. (v0.1.3)
- [x] **No scaffold clutter** — `example` command replaces shipped *.example;
  project.config.json consumed at render. (v0.1.4)
- [x] **Backlog is real** — `docs/planning/` seeded, feature on by default.
- [ ] **Agent-operable in consumer repos** — an agent can discover and create an
  overlay (and avoid `customize-opencode`/editing `.opencode/`) from in-repo
  guidance alone. (P1-OVERLAY-001/002)
- [ ] **Overlay scaffolding** — `vh-agent-harness overlay new <name>` wires a pack
  + profile in one step.
