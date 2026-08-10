# hermes-agent — Skills System (source packet)
**Source:** `refs/hermes-agent/` @ HEAD `005421d888a40865cc61d143ff77efd87a037a1e` on `main` (Nous Research, MIT, v0.20.0). Gitignored reference transport in this repo; NOT this repo's own code.
**Method:** files read in BODY off local disk; giant files sampled at structurally important sections, not dumped whole.
**Status:** NEUTRAL study. Borrow/reject verdicts characterize fit vs vh-agent-harness discipline (static-Go, small-boundary, token-stable, overlay-based, narrow-waist); they are NOT commands to change this repo.

> See `hermes-agent-internals.md` for the consolidated study and the cross-check corrections.

## §6 Skills system (slice 2).
- **184 skills: 71 bundled (`skills/`) + 113 optional (`optional-skills/`).** Optional installed via `hermes skills install official/<cat>/<skill>`; NOT active by default. Frontmatter: name, description(≤60ch), version, author, license, platforms, metadata.hermes.{tags,category,related_skills,config}. Less-common: prerequisites(commands/env_vars/pip), dependencies, required_environment_variables, triggers (only 4 skills), toolsets (only 2). *(Note: The full 184-skill catalog lives in scratch `tmp/agent-runs/researcher-skills/frontmatter_dump.{txt,json}`)* (source=refs/hermes-agent/tools/skills_tool.py, confidence=high, type=fact)
- Progressive disclosure `tools/skills_tool.py`: Level-1 `skills_list` (`:786`) returns {name,description,category}+hint; Level-2 `skill_view` (`:1049`) returns full body/linked file; path-traversal-validated. Discovery reads first 4000 chars/SKILL.md; hard platform gate (`:250`) vs offer-time env gate (`:260`); dedup by name; scan-signature cached+TTL. (source=refs/hermes-agent/tools/skills_tool.py:786, confidence=high, type=fact)
- **View-fingerprint dedup** (`:1910-1998`): stats (mtime+size); exempts SETUP_NEEDED; cleared on context compression; ~286k tokens saved/400k-msg window. Readiness enum (`:225`) AVAILABLE/SETUP_NEEDED/UNSUPPORTED. (source=refs/hermes-agent/tools/skills_tool.py:1910, confidence=high, type=fact)
- Write gate (`skill_manager_tool.py:1402`→`write_approval.py:253`): GateDecision allow/blocked/stage; **skills ALWAYS stage**; human reviews via `/skills pending`; approval replays via `apply_skill_pending` w/ bypass contextvar. (source=refs/hermes-agent/tools/skill_manager_tool.py:1402, confidence=high, type=fact)
- Install security gate `tools/skills_guard.py`: trust levels agent-created/builtin/trusted/community × verdict safe/caution/dangerous → INSTALL_POLICY allow/ask/block; `--force` cannot override dangerous+community/trusted. (source=refs/hermes-agent/tools/skills_guard.py, confidence=high, type=fact)
- Curator `agent/curator.py:306,439`: active→stale→archived; archive is MAX-destructive (never delete, recoverable by mv to `~/.hermes/skills/.archive/`); only touches `created_by:agent`; pinned+referenced exempt. (source=refs/hermes-agent/agent/curator.py:306, confidence=high, type=fact)
- Skills Hub `tools/skills_hub.py`: ~9 SkillSource subclasses (incl. OptionalSkillSource serving in-repo optional-skills/ at builtin trust); quarantine-then-install (`:3660`→`install_from_quarantine:3711`). (source=refs/hermes-agent/tools/skills_hub.py:3660, confidence=high, type=fact)
- **[cross-check CONFIRMED] Origin-hash sync** `tools/skills_sync.py:113-197,675-950`: three-way sync — `_read_manifest`=`{skill:origin_hash}`; update ONLY when user_hash==origin_hash (`:870`); divergence→append user_modified, skip, NEVER clobber (`:862`); user-deleted→respected (`:914-916`); upstream-removed→only `del manifest[name]`, on-disk NOT deleted (`:918-921`). Hermes's own term is "user-modified, skipping" (the "ownership transfer" framing is the cross-check author's gloss; mechanism exactly as claimed). (source=refs/hermes-agent/tools/skills_sync.py:113, confidence=high, type=fact)

## Verification
| Claim | Verifying command/read | Verified |
|-------|------------------------|----------|
| 184 skills (71 bundled, 113 optional) | File system read pass | yes |
| Skills ALWAYS stage | Structural code read pass | yes |
| View-fingerprint dedup mechanism | Structural code read pass | yes |

## Contradictions
None detected.

## Could-not-verify flags
- Giant files were structurally sampled; exhaustive line-by-line verification of all edge-case conditionals was not performed. (confidence=medium, type=inference)