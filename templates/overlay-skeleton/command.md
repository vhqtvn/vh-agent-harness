---
description: TODO one line: what /__UNIT_NAME__ does
agent: coordination
subtask: false
---

TODO: instructions the agent runs when `/__UNIT_NAME__` is invoked.

Renders into `.opencode/commands/__UNIT_NAME__.md` (ownership
`overlay_extension`) while this pack is selected under `overlays:` in
vh-harness-profile.yml. Slash commands are auto-discovered from
`.opencode/commands/`; no `opencode-append.jsonc` entry is needed for a command.
Change `agent:` / `subtask:` above to route the command to the right agent.
