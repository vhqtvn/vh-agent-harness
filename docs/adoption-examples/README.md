# Adoption examples

This directory holds **reference overlay packs** that show how a specific repo
would *adopt* the harness. They are NOT shipped as generic overlay packs: as of
the 2026-06-25 pre-publish clearance the harness ships **no** overlay packs.
`templates/overlays/` is empty by operator decision (the browser/web tooling is
not shipping yet); the `internal/overlay/` infrastructure stays in place as the
forward mechanism, so the day a pack ships it is dropped under
`templates/overlays/<pack>/` and listed automatically.

Each subdirectory here mirrors the on-disk layout an adopting repo would place
under `templates/overlays/<pack>/` (or vendor into its own fork of this harness)
and then select via `vh-harness-profile.yml: overlays:[<pack>]`.

| Example | What it illustrates |
| --- | --- |
| [`web/`](web/) | A generic web/frontend overlay (browser-qa + web-builder agents, browser-smoke/frontend/web-stack-up/browser-view commands, web-dev-loop + web-fixtures skills, plus the opencode-append / callable-graph-snippet / permission-pack merge descriptors). Demonstrates an overlay whose own tool vocabulary (Playwright, web-builder) is legitimate domain for the pack. |

To use one as the basis for a real overlay pack, copy the subdirectory into
`templates/overlays/<your-pack>/`, rename the pack to match your repo, and adjust
the agent/command/skill names and permission descriptors to match your domain.
