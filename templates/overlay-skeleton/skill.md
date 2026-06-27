---
name: __UNIT_NAME__
description: TODO one line: when to load this skill
compatibility: opencode
---

# __UNIT_NAME__

TODO: workflow instructions this skill carries. Describe when the agent should
load it and the steps it prescribes.

Renders into `.opencode/skills/__UNIT_NAME__/SKILL.md` (ownership
`overlay_extension`) while this pack is selected under `overlays:` in
vh-harness-profile.yml. Skills are auto-discovered from `.opencode/skills/`; no
`opencode-append.jsonc` entry is needed for a skill.
